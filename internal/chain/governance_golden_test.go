package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/membership"
)

type governanceLogical struct {
	Choice     string `json:"choice"`
	Expected   string `json:"expected"`
	Height     int64  `json:"height"`
	Kind       string `json:"kind"`
	ProposalID string `json:"proposal_id"`
	Proposer   string `json:"proposer"`
	Requested  string `json:"requested"`
	Submitter  string `json:"submitter"`
	Target     string `json:"target"`
	Voter      string `json:"voter"`
}

type governanceTransactionGolden struct {
	FixtureNotice  string            `json:"fixture_notice"`
	CanonicalCBOR  string            `json:"canonical_cbor_hex"`
	ExpectedCode   string            `json:"expected_code"`
	ExpectedHash   string            `json:"expected_state_hash"`
	ExpectedStatus string            `json:"expected_status"`
	Logical        governanceLogical `json:"logical"`
	ProposalID     string            `json:"proposal_id"`
	TxID           string            `json:"tx_id"`
}

type governanceStateGolden struct {
	FixtureNotice string `json:"fixture_notice"`
	CanonicalCBOR string `json:"canonical_snapshot_cbor_hex"`
	Logical       string `json:"logical"`
	SHA256        string `json:"sha256_hex"`
	StateHash     string `json:"state_hash"`
}

func loadGovernanceGolden[T any](t *testing.T, name string) T {
	t.Helper()
	raw, err := os.ReadFile("../governance/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var fixture T
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func requireGovernanceTransactionGolden(t *testing.T, fixture governanceTransactionGolden, tx Transaction, receipt Receipt) {
	t.Helper()
	if fixture.FixtureNotice != publicFixtureNotice {
		t.Fatal("governance fixture safety notice changed")
	}
	raw, err := tx.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(raw) != fixture.CanonicalCBOR {
		t.Fatal("governance canonical transaction differs from literal fixture")
	}
	txID, err := tx.ID()
	if err != nil {
		t.Fatal(err)
	}
	if txID.String() != fixture.TxID || receipt.TxID.String() != fixture.TxID || string(receipt.Code) != fixture.ExpectedCode || receipt.StateHash.String() != fixture.ExpectedHash {
		t.Fatalf("governance transaction result differs from fixture: tx=%s receipt=%+v", txID.String(), receipt)
	}
}

func TestGovernanceGoldenVectors(t *testing.T) {
	scenario := setupGovernanceState(t)

	proposalFixture := loadGovernanceGolden[governanceTransactionGolden](t, "governance_proposal_golden.json")
	proposal := membershipProposalTx(t, scenario, 0, 100)
	state, proposalReceipt := applyAt(t, scenario.state, proposal, proposalFixture.Logical.Height)
	if proposalFixture.Logical.Kind != string(governance.MembershipChange) || proposalFixture.Logical.Proposer != scenario.proofs[0].IdentityID.String() ||
		proposalFixture.Logical.Target != scenario.proofs[3].IdentityID.String() || proposalFixture.Logical.Expected != string(membership.Pending) ||
		proposalFixture.Logical.Requested != string(membership.Active) || proposalFixture.ProposalID != proposal.GovernanceProposal.ProposalID.String() {
		t.Fatal("logical governance proposal differs from fixture")
	}
	requireGovernanceTransactionGolden(t, proposalFixture, proposal, proposalReceipt)

	voteFixture := loadGovernanceGolden[governanceTransactionGolden](t, "governance_vote_golden.json")
	vote := voteTx(t, proposal.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0], governance.Yes)
	state, voteReceipt := applyAt(t, state, vote, voteFixture.Logical.Height)
	if voteFixture.Logical.ProposalID != proposal.GovernanceProposal.ProposalID.String() || voteFixture.Logical.Voter != scenario.proofs[0].IdentityID.String() || voteFixture.Logical.Choice != string(governance.Yes) {
		t.Fatal("logical governance vote differs from fixture")
	}
	requireGovernanceTransactionGolden(t, voteFixture, vote, voteReceipt)

	for i := 1; i < 3; i++ {
		state, _ = applyAt(t, state, voteTx(t, proposal.GovernanceProposal.ProposalID, scenario.proofs[i].IdentityID, scenario.keys[i], governance.Yes), int64(i+2))
	}
	finalizeFixture := loadGovernanceGolden[governanceTransactionGolden](t, "governance_finalize_golden.json")
	finalize := actionTx(t, GovernanceFinalize, proposal.GovernanceProposal.ProposalID, scenario.proofs[0].IdentityID, scenario.keys[0])
	state, finalizeReceipt := applyAt(t, state, finalize, finalizeFixture.Logical.Height)
	if finalizeFixture.Logical.ProposalID != proposal.GovernanceProposal.ProposalID.String() || finalizeFixture.Logical.Submitter != scenario.proofs[0].IdentityID.String() ||
		state.Governance.Proposals[proposalFixture.ProposalID].Status != governance.ProposalStatus(finalizeFixture.ExpectedStatus) {
		t.Fatal("logical governance finalization differs from fixture")
	}
	requireGovernanceTransactionGolden(t, finalizeFixture, finalize, finalizeReceipt)

	stateFixture := loadGovernanceGolden[governanceStateGolden](t, "governance_state_golden.json")
	canonical, err := state.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	wantCanonical, err := hex.DecodeString(stateFixture.CanonicalCBOR)
	if err != nil {
		t.Fatal(err)
	}
	if stateFixture.FixtureNotice != publicFixtureNotice || !bytes.Equal(canonical, wantCanonical) {
		t.Fatal("canonical governance state differs from literal fixture")
	}
	digest := sha256.Sum256(canonical)
	hash, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(digest[:]) != stateFixture.SHA256 || hash.String() != stateFixture.StateHash || hash.String() != finalizeReceipt.StateHash.String() {
		t.Fatal("governance state digest differs from fixture")
	}
}
