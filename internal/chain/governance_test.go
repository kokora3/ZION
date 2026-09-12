package chain

import (
	"bytes"
	"crypto/ed25519"
	"reflect"
	"sort"
	"testing"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

type governanceTestState struct {
	state  State
	keys   []ed25519.PrivateKey
	proofs []identity.IdentityGenesisProof
}

func governanceKey(start byte) ed25519.PrivateKey { return deterministicKey(start) }

func setupGovernanceState(t *testing.T) governanceTestState {
	t.Helper()
	result := governanceTestState{state: Genesis(protocol.Alpha1NetworkID)}
	for i, start := range []byte{0, 32, 64, 96} {
		key := governanceKey(start)
		tx, proof := identityTransaction(t, key, protocol.ProtocolTimestamp(i+1))
		var err error
		result.state, _, err = Apply(result.state, tx)
		if err != nil {
			t.Fatal(err)
		}
		result.keys = append(result.keys, key)
		result.proofs = append(result.proofs, proof)
	}
	validators := make([]governance.Validator, 4)
	for i := range validators {
		publicKey := bytes.Repeat([]byte{byte(160 + i)}, ed25519.PublicKeySize)
		validators[i] = governance.Validator{Operator: result.proofs[i%3].IdentityID,
			PublicKey: publicKey, Power: governance.ValidatorPower}
	}
	var err error
	result.state, err = BootstrapGovernance(result.state,
		[]identity.IdentityID{result.proofs[0].IdentityID, result.proofs[1].IdentityID, result.proofs[2].IdentityID}, validators)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func membershipProposalTx(t testing.TB, scenario governanceTestState, proposer int, createdAt protocol.ProtocolTimestamp) Transaction {
	t.Helper()
	body := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		Kind: governance.MembershipChange, Proposer: scenario.proofs[proposer].IdentityID, CreatedAt: createdAt,
		Membership: &governance.MembershipChangePayload{Target: scenario.proofs[3].IdentityID,
			Expected: membership.Pending, Requested: membership.Active}}
	authorization, err := governance.CreateProposal(body, scenario.keys[proposer])
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: GovernanceProposal, GovernanceProposal: &authorization}
}

func voteTx(t testing.TB, proposalID governance.ProposalID, voter identity.IdentityID, key ed25519.PrivateKey, choice governance.VoteChoice) Transaction {
	t.Helper()
	authorization, err := governance.CreateVote(governance.VoteBody{SchemaVersion: governance.Schema,
		NetworkID: protocol.Alpha1NetworkID, ProposalID: proposalID, Voter: voter, Choice: choice}, key)
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: GovernanceVote, GovernanceVote: &authorization}
}

func actionTx(t testing.TB, transactionType TransactionType, proposalID governance.ProposalID, submitter identity.IdentityID, key ed25519.PrivateKey) Transaction {
	t.Helper()
	body := governance.ActionBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		ProposalID: proposalID, Submitter: submitter}
	var authorization governance.ActionAuthorization
	var err error
	if transactionType == GovernanceFinalize {
		authorization, err = governance.CreateFinalize(body, key)
	} else {
		authorization, err = governance.CreateExecute(body, key)
	}
	if err != nil {
		t.Fatal(err)
	}
	tx := Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: transactionType}
	if transactionType == GovernanceFinalize {
		tx.GovernanceFinalize = &authorization
	} else {
		tx.GovernanceExecute = &authorization
	}
	return tx
}

func applyAt(t testing.TB, state State, tx Transaction, height int64) (State, Receipt) {
	t.Helper()
	next, receipt, err := ApplyWithContext(state, tx, ExecutionContext{Height: height})
	if err != nil {
		t.Fatalf("apply %s at %d: receipt=%+v err=%v", tx.Type, height, receipt, err)
	}
	return next, receipt
}

func rejectAt(t testing.TB, state State, tx Transaction, height int64, expected Code) {
	t.Helper()
	before, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	next, receipt, err := ApplyWithContext(state, tx, ExecutionContext{Height: height})
	if err == nil || receipt.Code != expected {
		t.Fatalf("rejection code=%s err=%v, want %s", receipt.Code, err, expected)
	}
	after, hashErr := next.Hash()
	if hashErr != nil || before.String() != after.String() || receipt.StateHash.String() != before.String() {
		t.Fatal("rejected governance transaction mutated canonical state")
	}
}

func decideMembership(t *testing.T, choices []governance.VoteChoice) (governanceTestState, State, governance.ProposalID) {
	t.Helper()
	scenario := setupGovernanceState(t)
	proposalTx := membershipProposalTx(t, scenario, 0, 100)
	state, _ := applyAt(t, scenario.state, proposalTx, 1)
	proposalID := proposalTx.GovernanceProposal.ProposalID
	for i, choice := range choices {
		state, _ = applyAt(t, state, voteTx(t, proposalID, scenario.proofs[i].IdentityID, scenario.keys[i], choice), int64(i+2))
	}
	state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, proposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 11)
	return scenario, state, proposalID
}

func TestGovernanceMembershipEndToEnd(t *testing.T) {
	scenario, state, proposalID := decideMembership(t, []governance.VoteChoice{governance.Yes, governance.Yes, governance.Yes})
	if state.Governance.Proposals[proposalID.String()].Status != governance.Approved {
		t.Fatal("three YES votes did not approve proposal")
	}
	state, _ = applyAt(t, state, actionTx(t, GovernanceExecute, proposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 12)
	if state.Memberships[scenario.proofs[3].IdentityID.String()] != membership.Active ||
		state.Governance.Proposals[proposalID.String()].Status != governance.Executed {
		t.Fatal("approved membership proposal did not execute")
	}
	rejectAt(t, state, actionTx(t, GovernanceExecute, proposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 13, AlreadyExecuted)
}

func TestGovernanceDecisionBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		choices []governance.VoteChoice
		want    governance.ProposalStatus
	}{
		{"two yes one no", []governance.VoteChoice{governance.Yes, governance.Yes, governance.No}, governance.Rejected},
		{"two participants", []governance.VoteChoice{governance.Yes, governance.Yes}, governance.Expired},
		{"abstain participation", []governance.VoteChoice{governance.Yes, governance.Yes, governance.Abstain}, governance.Approved},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, state, proposalID := decideMembership(t, test.choices)
			if got := state.Governance.Proposals[proposalID.String()].Status; got != test.want {
				t.Fatalf("status=%s, want %s", got, test.want)
			}
		})
	}
}

func TestDuplicateVoteAndRotationCannotResetIdentityVote(t *testing.T) {
	scenario := setupGovernanceState(t)
	proposalTx := membershipProposalTx(t, scenario, 0, 100)
	state, _ := applyAt(t, scenario.state, proposalTx, 1)
	proposalID := proposalTx.GovernanceProposal.ProposalID
	first := voteTx(t, proposalID, scenario.proofs[0].IdentityID, scenario.keys[0], governance.Yes)
	state, _ = applyAt(t, state, first, 2)
	rejectAt(t, state, voteTx(t, proposalID, scenario.proofs[0].IdentityID, scenario.keys[0], governance.No), 3, DuplicateVote)

	newKey := governanceKey(128)
	rotation := rotationTransaction(t, scenario.proofs[0], scenario.keys[0], newKey, 1)
	state, _ = applyAt(t, state, rotation, 3)
	rejectAt(t, state, voteTx(t, proposalID, scenario.proofs[0].IdentityID, newKey, governance.No), 4, DuplicateVote)

	badProposal := membershipProposalTx(t, scenario, 0, 101)
	rejectAt(t, state, badProposal, 5, KeyNotActive)
	if len(state.Governance.Proposals[proposalID.String()].Votes) != 1 ||
		state.Governance.Proposals[proposalID.String()].Votes[scenario.proofs[0].IdentityID.String()].Choice != governance.Yes {
		t.Fatal("duplicate vote changed original canonical vote")
	}
}

func TestElectorateSnapshotAndCurrentActiveEligibility(t *testing.T) {
	scenario := setupGovernanceState(t)
	proposalTx := membershipProposalTx(t, scenario, 0, 100)
	state, _ := applyAt(t, scenario.state, proposalTx, 1)
	proposalID := proposalTx.GovernanceProposal.ProposalID
	state.Memberships[scenario.proofs[3].IdentityID.String()] = membership.Active
	rejectAt(t, state, voteTx(t, proposalID, scenario.proofs[3].IdentityID, scenario.keys[3], governance.Yes), 2, NotEligible)
	state.Memberships[scenario.proofs[2].IdentityID.String()] = membership.Suspended
	rejectAt(t, state, voteTx(t, proposalID, scenario.proofs[2].IdentityID, scenario.keys[2], governance.Yes), 2, NotEligible)
	state.Memberships[scenario.proofs[2].IdentityID.String()] = membership.Revoked
	rejectAt(t, state, voteTx(t, proposalID, scenario.proofs[2].IdentityID, scenario.keys[2], governance.Yes), 2, NotEligible)
}

func TestGovernancePrematureFinalizeAndStaleExecuteAreAtomic(t *testing.T) {
	scenario := setupGovernanceState(t)
	firstTx := membershipProposalTx(t, scenario, 0, 100)
	secondTx := membershipProposalTx(t, scenario, 1, 101)
	state, _ := applyAt(t, scenario.state, firstTx, 1)
	state, _ = applyAt(t, state, secondTx, 1)
	rejectAt(t, state, actionTx(t, GovernanceFinalize, firstTx.GovernanceProposal.ProposalID,
		scenario.proofs[0].IdentityID, scenario.keys[0]), 10, NotReady)
	for i := 0; i < 3; i++ {
		state, _ = applyAt(t, state, voteTx(t, firstTx.GovernanceProposal.ProposalID, scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), int64(2+i))
		state, _ = applyAt(t, state, voteTx(t, secondTx.GovernanceProposal.ProposalID, scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), int64(5+i))
	}
	state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, firstTx.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 11)
	state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, secondTx.GovernanceProposal.ProposalID, scenario.proofs[1].IdentityID, scenario.keys[1]), 11)
	state, _ = applyAt(t, state, actionTx(t, GovernanceExecute, firstTx.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 12)
	rejectAt(t, state, actionTx(t, GovernanceExecute, secondTx.GovernanceProposal.ProposalID, scenario.proofs[1].IdentityID, scenario.keys[1]), 13, Stale)
}

func validatorProposalTx(t testing.TB, scenario governanceTestState, state State, action governance.ValidatorAction,
	validator governance.Validator, proposer int, createdAt protocol.ProtocolTimestamp) Transaction {
	t.Helper()
	hash, err := state.Governance.ValidatorSetHash()
	if err != nil {
		t.Fatal(err)
	}
	body := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		Kind: governance.ValidatorSetChange, Proposer: scenario.proofs[proposer].IdentityID, CreatedAt: createdAt,
		ValidatorSet: &governance.ValidatorSetChangePayload{Action: action, Validator: validator, ExpectedSetHash: hash}}
	authorization, err := governance.CreateProposal(body, scenario.keys[proposer])
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: GovernanceProposal, GovernanceProposal: &authorization}
}

func approveAndExecute(t testing.TB, scenario governanceTestState, state State, proposalTx Transaction, start int64) (State, Receipt) {
	t.Helper()
	state, _ = applyAt(t, state, proposalTx, start)
	proposalID := proposalTx.GovernanceProposal.ProposalID
	for i := 0; i < 3; i++ {
		state, _ = applyAt(t, state, voteTx(t, proposalID, scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), start+int64(i)+1)
	}
	state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, proposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), start+int64(governance.VotingPeriodBlocks))
	return applyAt(t, state, actionTx(t, GovernanceExecute, proposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), start+int64(governance.VotingPeriodBlocks)+1)
}

func TestValidatorSetGovernanceAddRemoveAndDirectRejection(t *testing.T) {
	scenario := setupGovernanceState(t)
	added := governance.Validator{Operator: scenario.proofs[0].IdentityID,
		PublicKey: bytes.Repeat([]byte{0xee}, ed25519.PublicKeySize), Power: governance.ValidatorPower}
	addTx := validatorProposalTx(t, scenario, scenario.state, governance.AddValidator, added, 0, 200)
	state, receipt := approveAndExecute(t, scenario, scenario.state, addTx, 1)
	if len(state.Governance.Validators) != 5 || len(receipt.ValidatorUpdates) != 1 || receipt.ValidatorUpdates[0].Power != 1 {
		t.Fatalf("validator ADD result state=%d receipt=%+v", len(state.Governance.Validators), receipt)
	}
	removeTx := validatorProposalTx(t, scenario, state, governance.RemoveValidator, added, 1, 201)
	state, receipt = approveAndExecute(t, scenario, state, removeTx, 20)
	if len(state.Governance.Validators) != 4 || len(receipt.ValidatorUpdates) != 1 || receipt.ValidatorUpdates[0].Power != 0 {
		t.Fatalf("validator REMOVE result state=%d receipt=%+v", len(state.Governance.Validators), receipt)
	}
	direct := Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: ValidatorSetChange}
	rejectAt(t, state, direct, 40, Unsupported)
}

func TestGovernanceCanonicalMapOrderAndReplay(t *testing.T) {
	scenario, expected, _ := decideMembership(t, []governance.VoteChoice{governance.Yes, governance.Yes, governance.Abstain})
	expectedBytes, _ := expected.CanonicalBytes()
	expectedHash, _ := expected.Hash()
	for run := 0; run < 100; run++ {
		_, state, _ := decideMembership(t, []governance.VoteChoice{governance.Yes, governance.Yes, governance.Abstain})
		actualBytes, _ := state.CanonicalBytes()
		actualHash, _ := state.Hash()
		if !bytes.Equal(actualBytes, expectedBytes) || actualHash.String() != expectedHash.String() {
			t.Fatalf("governance replay %d diverged", run)
		}
	}

	copyState := cloneState(expected)
	rebuiltProposals := make(map[string]governance.ProposalRecord)
	for key, proposal := range copyState.Governance.Proposals {
		rebuiltVotes := make(map[string]governance.VoteRecord)
		voteKeys := make([]string, 0, len(proposal.Votes))
		for voter := range proposal.Votes {
			voteKeys = append(voteKeys, voter)
		}
		for i := len(voteKeys) - 1; i >= 0; i-- {
			rebuiltVotes[voteKeys[i]] = proposal.Votes[voteKeys[i]]
		}
		proposal.Votes = rebuiltVotes
		rebuiltProposals[key] = proposal
	}
	copyState.Governance.Proposals = rebuiltProposals
	copyBytes, _ := copyState.CanonicalBytes()
	copyHash, _ := copyState.Hash()
	if !bytes.Equal(copyBytes, expectedBytes) || copyHash.String() != expectedHash.String() || reflect.DeepEqual(scenario.state.Governance, expected.Governance) {
		t.Fatal("governance canonical state depends on map insertion order")
	}
}

func TestProtocolUpgradeDecisionRecordsWithoutHotSwap(t *testing.T) {
	scenario := setupGovernanceState(t)
	body := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		Kind: governance.ProtocolUpgrade, Proposer: scenario.proofs[0].IdentityID, CreatedAt: 300,
		Upgrade: &governance.ProtocolUpgradePayload{TargetVersion: "0.2", ActivationHeight: 1000, MigrationIdentifier: "migration-0.2"}}
	authorization, err := governance.CreateProposal(body, scenario.keys[0])
	if err != nil {
		t.Fatal(err)
	}
	tx := Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: GovernanceProposal, GovernanceProposal: &authorization}
	state, _ := approveAndExecute(t, scenario, scenario.state, tx, 1)
	if state.Governance.Proposals[authorization.ProposalID.String()].Status != governance.Executed || state.ProtocolVersion != protocol.CurrentProtocolVersion {
		t.Fatal("protocol decision hot-swapped application protocol")
	}
}

func TestGovernanceAtomicityMatrix(t *testing.T) {
	t.Run("duplicate proposal", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		tx := membershipProposalTx(t, scenario, 0, 100)
		state, _ := applyAt(t, scenario.state, tx, 1)
		rejectAt(t, state, tx, 2, ProposalExists)
	})
	t.Run("invalid proposer membership", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		tx := membershipProposalTx(t, scenario, 0, 100)
		scenario.state.Memberships[scenario.proofs[0].IdentityID.String()] = membership.Suspended
		rejectAt(t, scenario.state, tx, 1, NotEligible)
	})
	t.Run("bad proposal signature", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		tx := membershipProposalTx(t, scenario, 0, 100)
		tx.GovernanceProposal.Signature.Bytes = append([]byte(nil), tx.GovernanceProposal.Signature.Bytes...)
		tx.GovernanceProposal.Signature.Bytes[0] ^= 1
		rejectAt(t, scenario.state, tx, 1, Invalid)
	})
	t.Run("unknown proposal", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		unknown := membershipProposalTx(t, scenario, 0, 101).GovernanceProposal.ProposalID
		rejectAt(t, scenario.state, voteTx(t, unknown, scenario.proofs[0].IdentityID, scenario.keys[0], governance.Yes), 2, ProposalNotFound)
	})
	t.Run("duplicate vote", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		proposal := membershipProposalTx(t, scenario, 0, 100)
		state, _ := applyAt(t, scenario.state, proposal, 1)
		vote := voteTx(t, proposal.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0], governance.Yes)
		state, _ = applyAt(t, state, vote, 2)
		rejectAt(t, state, voteTx(t, proposal.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0], governance.No), 3, DuplicateVote)
	})
	t.Run("non eligible voter", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		proposal := membershipProposalTx(t, scenario, 0, 100)
		state, _ := applyAt(t, scenario.state, proposal, 1)
		rejectAt(t, state, voteTx(t, proposal.GovernanceProposal.ProposalID, scenario.proofs[3].IdentityID, scenario.keys[3], governance.Yes), 2, NotEligible)
	})
	t.Run("bad vote signature", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		proposal := membershipProposalTx(t, scenario, 0, 100)
		state, _ := applyAt(t, scenario.state, proposal, 1)
		vote := voteTx(t, proposal.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0], governance.Yes)
		vote.GovernanceVote.Signature.Bytes = append([]byte(nil), vote.GovernanceVote.Signature.Bytes...)
		vote.GovernanceVote.Signature.Bytes[0] ^= 1
		rejectAt(t, state, vote, 2, Invalid)
	})
	t.Run("wrong network", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		tx := membershipProposalTx(t, scenario, 0, 100)
		tx.NetworkID = "wrong-network"
		rejectAt(t, scenario.state, tx, 1, WrongNetwork)
	})
	t.Run("canonical transaction too large", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		tx := membershipProposalTx(t, scenario, 0, 100)
		tx.GovernanceProposal.Signature.Bytes = make([]byte, MaxTransactionBytes)
		raw, err := tx.CanonicalBytes()
		if err != nil || len(raw) <= MaxTransactionBytes {
			t.Fatal("test transaction does not exceed the canonical protocol limit")
		}
		rejectAt(t, scenario.state, tx, 1, TooLarge)
	})
	t.Run("premature finalization", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		proposal := membershipProposalTx(t, scenario, 0, 100)
		state, _ := applyAt(t, scenario.state, proposal, 1)
		rejectAt(t, state, actionTx(t, GovernanceFinalize, proposal.GovernanceProposal.ProposalID,
			scenario.proofs[0].IdentityID, scenario.keys[0]), 10, NotReady)
	})
	t.Run("invalid validator payload", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		validator := governance.Validator{Operator: scenario.proofs[0].IdentityID,
			PublicKey: bytes.Repeat([]byte{0xee}, ed25519.PublicKeySize), Power: governance.ValidatorPower}
		tx := validatorProposalTx(t, scenario, scenario.state, governance.AddValidator, validator, 0, 200)
		tx.GovernanceProposal.Body.ValidatorSet.Validator.Power = 1000
		rejectAt(t, scenario.state, tx, 1, Invalid)
	})
	t.Run("unauthorized validator change", func(t *testing.T) {
		scenario := setupGovernanceState(t)
		rejectAt(t, scenario.state, Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
			Type: ValidatorSetChange}, 1, Unsupported)
	})
}

func TestRejectedAndExpiredProposalsCannotExecute(t *testing.T) {
	for _, test := range []struct {
		name    string
		choices []governance.VoteChoice
		status  governance.ProposalStatus
	}{
		{"rejected", []governance.VoteChoice{governance.Yes, governance.Yes, governance.No}, governance.Rejected},
		{"expired", []governance.VoteChoice{governance.Yes, governance.Yes}, governance.Expired},
	} {
		t.Run(test.name, func(t *testing.T) {
			scenario, state, proposalID := decideMembership(t, test.choices)
			if state.Governance.Proposals[proposalID.String()].Status != test.status {
				t.Fatalf("proposal status is not %s", test.status)
			}
			rejectAt(t, state, actionTx(t, GovernanceExecute, proposalID, scenario.proofs[0].IdentityID, scenario.keys[0]), 12, ProposalClosed)
		})
	}
}

func TestValidatorExecutionRequiresActiveOperatorAndMinimumSet(t *testing.T) {
	scenario := setupGovernanceState(t)
	pendingOperator := governance.Validator{Operator: scenario.proofs[3].IdentityID,
		PublicKey: bytes.Repeat([]byte{0xed}, ed25519.PublicKeySize), Power: governance.ValidatorPower}
	proposal := validatorProposalTx(t, scenario, scenario.state, governance.AddValidator, pendingOperator, 0, 200)
	state, _ := applyAt(t, scenario.state, proposal, 1)
	for i := 0; i < 3; i++ {
		state, _ = applyAt(t, state, voteTx(t, proposal.GovernanceProposal.ProposalID,
			scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), int64(i+2))
	}
	state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, proposal.GovernanceProposal.ProposalID,
		scenario.proofs[0].IdentityID, scenario.keys[0]), 11)
	rejectAt(t, state, actionTx(t, GovernanceExecute, proposal.GovernanceProposal.ProposalID,
		scenario.proofs[0].IdentityID, scenario.keys[0]), 12, NotEligible)

	state = scenario.state
	for removal := 0; removal < 2; removal++ {
		validator := state.Governance.Validators[governance.ValidatorKey(scenarioValidatorKey(state, removal))]
		remove := validatorProposalTx(t, scenario, state, governance.RemoveValidator, validator, removal, protocol.ProtocolTimestamp(300+removal))
		if removal == 0 {
			state, _ = approveAndExecute(t, scenario, state, remove, 20)
			continue
		}
		state, _ = applyAt(t, state, remove, 40)
		for i := 0; i < 3; i++ {
			state, _ = applyAt(t, state, voteTx(t, remove.GovernanceProposal.ProposalID,
				scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), int64(41+i))
		}
		state, _ = applyAt(t, state, actionTx(t, GovernanceFinalize, remove.GovernanceProposal.ProposalID,
			scenario.proofs[0].IdentityID, scenario.keys[0]), 50)
		rejectAt(t, state, actionTx(t, GovernanceExecute, remove.GovernanceProposal.ProposalID,
			scenario.proofs[0].IdentityID, scenario.keys[0]), 51, Invalid)
	}
}

func scenarioValidatorKey(state State, index int) []byte {
	keys := make([]string, 0, len(state.Governance.Validators))
	for key := range state.Governance.Validators {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return state.Governance.Validators[keys[index]].PublicKey
}

func TestMigrationDecisionRecordsWithoutExportImport(t *testing.T) {
	scenario := setupGovernanceState(t)
	body := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		Kind: governance.Migration, Proposer: scenario.proofs[0].IdentityID, CreatedAt: 500,
		Migration: &governance.MigrationPayload{TargetNetwork: "zion-alpha-2", TargetVersion: "0.2", MigrationIdentifier: "alpha-reset-1"}}
	authorization, err := governance.CreateProposal(body, scenario.keys[0])
	if err != nil {
		t.Fatal(err)
	}
	tx := Transaction{SchemaVersion: TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: GovernanceProposal, GovernanceProposal: &authorization}
	state, _ := approveAndExecute(t, scenario, scenario.state, tx, 1)
	if state.Governance.Proposals[authorization.ProposalID.String()].Status != governance.Executed ||
		state.NetworkID != protocol.Alpha1NetworkID || state.ProtocolVersion != protocol.CurrentProtocolVersion {
		t.Fatal("migration decision performed an unsafe export, import, or network switch")
	}
}

func TestGovernanceSnapshotIgnoresProposalVoteMapAndAllocationOrder(t *testing.T) {
	scenario := setupGovernanceState(t)
	state := scenario.state
	proposals := make([]governance.ProposalID, 0, 3)
	for i := 0; i < 3; i++ {
		tx := membershipProposalTx(t, scenario, i, protocol.ProtocolTimestamp(600+i))
		state, _ = applyAt(t, state, tx, int64(i+1))
		proposals = append(proposals, tx.GovernanceProposal.ProposalID)
	}
	choices := []governance.VoteChoice{governance.Yes, governance.No, governance.Abstain}
	for _, proposalID := range proposals {
		for voter, choice := range choices {
			state, _ = applyAt(t, state, voteTx(t, proposalID, scenario.proofs[voter].IdentityID,
				scenario.keys[voter], choice), 4)
		}
	}
	wantBytes, err := state.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}

	independent := cloneState(state)
	reordered := make(map[string]governance.ProposalRecord, len(independent.Governance.Proposals))
	for i := len(proposals) - 1; i >= 0; i-- {
		key := proposals[i].String()
		record := independent.Governance.Proposals[key]
		voteKeys := make([]string, 0, len(record.Votes))
		for voter := range record.Votes {
			voteKeys = append(voteKeys, voter)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(voteKeys)))
		votes := make(map[string]governance.VoteRecord, len(voteKeys))
		for _, voter := range voteKeys {
			vote := record.Votes[voter]
			vote.KeyID.Digest = append(make([]byte, 0, len(vote.KeyID.Digest)+19), vote.KeyID.Digest...)
			votes[voter] = vote
		}
		record.Votes = votes
		record.Electorate = append(make([]identity.IdentityID, 0, len(record.Electorate)+23), record.Electorate...)
		reordered[key] = record
	}
	independent.Governance.Proposals = reordered
	validatorKeys := make([]string, 0, len(independent.Governance.Validators))
	for key := range independent.Governance.Validators {
		validatorKeys = append(validatorKeys, key)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(validatorKeys)))
	validators := make(map[string]governance.Validator, len(validatorKeys))
	for _, key := range validatorKeys {
		validator := independent.Governance.Validators[key]
		validator.PublicKey = append(make([]byte, 0, len(validator.PublicKey)+29), validator.PublicKey...)
		validators[key] = validator
	}
	independent.Governance.Validators = validators

	gotBytes, err := independent.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	gotHash, err := independent.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wantBytes, gotBytes) || wantHash.String() != gotHash.String() {
		t.Fatal("proposal/vote/validator map order or allocation changed canonical governance state")
	}
}
