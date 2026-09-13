package chain

import (
	"fmt"
	"math"
	"sort"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
)

// BootstrapGovernance is the one-time, auditable StateSchemaV1 to
// StateSchemaV2 migration. It has no transaction or runtime-admin equivalent.
func BootstrapGovernance(state State, activeMembers []identity.IdentityID, validators []governance.Validator) (State, error) {
	if state.SchemaVersion != StateSchemaV1 || state.Governance != nil {
		return state, fmt.Errorf("governance bootstrap requires state schema v1")
	}
	if len(activeMembers) < int(governance.MinimumUniqueVoters) || len(activeMembers) > governance.MaxElectorateSize {
		return state, fmt.Errorf("invalid initial governance electorate size")
	}
	next := cloneState(state)
	active := make(map[string]struct{}, len(activeMembers))
	for _, memberID := range activeMembers {
		key := memberID.String()
		if key == "" {
			return state, fmt.Errorf("invalid bootstrap member")
		}
		if _, duplicate := active[key]; duplicate {
			return state, fmt.Errorf("duplicate bootstrap member")
		}
		if _, exists := next.Identities[key]; !exists {
			return state, fmt.Errorf("bootstrap member identity not found")
		}
		if next.Memberships[key] != membership.Pending {
			return state, fmt.Errorf("bootstrap member is not PENDING")
		}
		active[key] = struct{}{}
		next.Memberships[key] = membership.Active
	}
	for _, validator := range validators {
		if _, ok := active[validator.Operator.String()]; !ok {
			return state, fmt.Errorf("initial validator operator is not an ACTIVE bootstrap member")
		}
	}
	governanceState, err := governance.NewState(validators)
	if err != nil {
		return state, err
	}
	next.SchemaVersion = StateSchemaV2
	next.Governance = &governanceState
	if _, err := next.Hash(); err != nil {
		return state, fmt.Errorf("invalid governance bootstrap state: %w", err)
	}
	return next, nil
}

func applyGovernance(original, next State, tx Transaction, txID TxID, context ExecutionContext) (State, Receipt, error) {
	if context.Height <= 0 {
		return reject(original, txID, tx.Type, Invalid, "governance requires positive consensus height", nil)
	}
	if (next.SchemaVersion != StateSchemaV2 && next.SchemaVersion != StateSchemaV3) || next.Governance == nil {
		return reject(original, txID, tx.Type, GovernanceUnavailable, "governance is not bootstrapped", nil)
	}
	if transactionPayloadCount(tx) != 1 {
		return reject(original, txID, tx.Type, Invalid, "invalid governance transaction payload", nil)
	}
	var updates []governance.ValidatorUpdate
	var err error
	switch tx.Type {
	case GovernanceProposal:
		err = applyProposal(next, tx, context)
	case GovernanceVote:
		err = applyVote(next, tx, context)
	case GovernanceFinalize:
		err = applyFinalize(next, tx, context)
	case GovernanceExecute:
		updates, err = applyExecute(next, tx, context)
	default:
		err = fmt.Errorf("unsupported governance transaction")
	}
	if err != nil {
		code := governanceErrorCode(err)
		return reject(original, txID, tx.Type, code, "governance transaction rejected", err)
	}
	stateHash, err := next.Hash()
	if err != nil {
		return reject(original, txID, tx.Type, Invalid, "invalid resulting governance state", err)
	}
	return next, Receipt{TxID: txID, Type: tx.Type, Code: OK, StateHash: stateHash, ValidatorUpdates: updates}, nil
}

type governanceFailure struct {
	code Code
	text string
}

func (failure governanceFailure) Error() string { return failure.text }

func fail(code Code, text string) error { return governanceFailure{code: code, text: text} }

func governanceErrorCode(err error) Code {
	if failure, ok := err.(governanceFailure); ok {
		return failure.code
	}
	return Invalid
}

func authorizedGovernanceKey(state State, memberID identity.IdentityID, signature identity.Signature) (identity.PublicKey, error) {
	memberKey := memberID.String()
	stored, exists := state.Identities[memberKey]
	if !exists {
		return identity.PublicKey{}, fail(NotFound, "governance identity not found")
	}
	if state.Memberships[memberKey] != membership.Active {
		return identity.PublicKey{}, fail(NotEligible, "governance identity is not ACTIVE")
	}
	keyID := signature.KeyID.String()
	publicKey, exists := stored.Keys[keyID]
	if !exists || !stored.Active[keyID] {
		return identity.PublicKey{}, fail(KeyNotActive, "governance signing key is not active")
	}
	return publicKey, nil
}

func applyProposal(state State, tx Transaction, context ExecutionContext) error {
	if tx.GovernanceProposal == nil {
		return fail(Invalid, "missing governance proposal")
	}
	proposal := *tx.GovernanceProposal
	if proposal.Body.NetworkID != tx.NetworkID {
		return fail(WrongNetwork, "proposal network mismatch")
	}
	publicKey, err := authorizedGovernanceKey(state, proposal.Body.Proposer, proposal.Signature)
	if err != nil {
		return err
	}
	if err := governance.VerifyProposal(proposal, publicKey); err != nil {
		return fail(Invalid, "invalid proposal signature")
	}
	storedProposalID, err := proposal.Body.ID()
	if err != nil {
		return fail(Invalid, "invalid proposal body")
	}
	proposalKey := storedProposalID.String()
	if _, exists := state.Governance.Proposals[proposalKey]; exists {
		return fail(ProposalExists, "proposal already exists")
	}
	if (proposal.Body.Kind == governance.ResearchAdmission || proposal.Body.Kind == governance.ResourceAdmission) && state.SchemaVersion != StateSchemaV3 {
		return fail(GovernanceUnavailable, "registry proposal requires state schema v3")
	}
	if proposal.Body.Membership != nil {
		target := proposal.Body.Membership.Target.String()
		if _, exists := state.Identities[target]; !exists {
			return fail(NotFound, "membership target not found")
		}
		if state.Memberships[target] != proposal.Body.Membership.Expected {
			return fail(Stale, "membership proposal is already stale")
		}
	}
	if proposal.Body.ValidatorSet != nil {
		currentHash, err := state.Governance.ValidatorSetHash()
		if err != nil || !governance.EqualHash(currentHash, proposal.Body.ValidatorSet.ExpectedSetHash) {
			return fail(Stale, "validator-set proposal is stale")
		}
	}
	electorateKeys := make([]string, 0, len(state.Memberships))
	for memberID, status := range state.Memberships {
		if status == membership.Active {
			electorateKeys = append(electorateKeys, memberID)
		}
	}
	if len(electorateKeys) > int(state.Governance.Policy.MaxElectorate) {
		return fail(Invalid, "governance electorate exceeds protocol limit")
	}
	sort.Strings(electorateKeys)
	electorate := make([]identity.IdentityID, 0, len(electorateKeys))
	for _, key := range electorateKeys {
		electorate = append(electorate, state.Identities[key].ID)
	}
	if context.Height > math.MaxInt64-int64(state.Governance.Policy.VotingPeriodBlocks) {
		return fail(Invalid, "voting height overflow")
	}
	state.Governance.Proposals[proposalKey] = governance.ProposalRecord{
		ID: storedProposalID, Body: governance.CloneProposalBody(proposal.Body), Status: governance.Open,
		StartHeight: context.Height, EndHeight: context.Height + int64(state.Governance.Policy.VotingPeriodBlocks),
		Electorate: electorate, Votes: make(map[string]governance.VoteRecord),
	}
	return nil
}

func applyVote(state State, tx Transaction, context ExecutionContext) error {
	if tx.GovernanceVote == nil {
		return fail(Invalid, "missing governance vote")
	}
	vote := *tx.GovernanceVote
	if vote.Body.NetworkID != tx.NetworkID {
		return fail(WrongNetwork, "vote network mismatch")
	}
	if err := vote.Body.Validate(); err != nil {
		return fail(Invalid, "invalid vote body")
	}
	proposalKey := vote.Body.ProposalID.String()
	record, exists := state.Governance.Proposals[proposalKey]
	if !exists {
		return fail(ProposalNotFound, "proposal not found")
	}
	if record.Status != governance.Open || context.Height >= record.EndHeight {
		return fail(ProposalClosed, "proposal is not open for voting")
	}
	if !governance.ContainsIdentity(record.Electorate, vote.Body.Voter) {
		return fail(NotEligible, "voter is outside proposal electorate")
	}
	voterKey := vote.Body.Voter.String()
	if _, duplicate := record.Votes[voterKey]; duplicate {
		return fail(DuplicateVote, "identity already voted")
	}
	publicKey, err := authorizedGovernanceKey(state, vote.Body.Voter, vote.Signature)
	if err != nil {
		return err
	}
	if err := governance.VerifyVote(vote, publicKey); err != nil {
		return fail(Invalid, "invalid vote signature")
	}
	storedVoter := state.Identities[voterKey].ID
	storedKeyID, err := identity.DeriveKeyID(publicKey)
	if err != nil {
		return fail(Invalid, "invalid vote key")
	}
	record.Votes[voterKey] = governance.VoteRecord{Voter: storedVoter, Choice: vote.Body.Choice, KeyID: storedKeyID}
	state.Governance.Proposals[proposalKey] = record
	return nil
}

func applyFinalize(state State, tx Transaction, context ExecutionContext) error {
	if tx.GovernanceFinalize == nil {
		return fail(Invalid, "missing governance finalization")
	}
	action := *tx.GovernanceFinalize
	if action.Body.NetworkID != tx.NetworkID {
		return fail(WrongNetwork, "finalization network mismatch")
	}
	publicKey, err := authorizedGovernanceKey(state, action.Body.Submitter, action.Signature)
	if err != nil {
		return err
	}
	if err := governance.VerifyFinalize(action, publicKey); err != nil {
		return fail(Invalid, "invalid finalization signature")
	}
	key := action.Body.ProposalID.String()
	record, exists := state.Governance.Proposals[key]
	if !exists {
		return fail(ProposalNotFound, "proposal not found")
	}
	if record.Status != governance.Open {
		return fail(ProposalClosed, "proposal already finalized")
	}
	if context.Height < record.EndHeight {
		return fail(NotReady, "proposal voting period has not ended")
	}
	record.Status = state.Governance.Policy.Decision(record.Tally())
	state.Governance.Proposals[key] = record
	return nil
}

func applyExecute(state State, tx Transaction, context ExecutionContext) ([]governance.ValidatorUpdate, error) {
	if tx.GovernanceExecute == nil {
		return nil, fail(Invalid, "missing governance execution")
	}
	action := *tx.GovernanceExecute
	if action.Body.NetworkID != tx.NetworkID {
		return nil, fail(WrongNetwork, "execution network mismatch")
	}
	publicKey, err := authorizedGovernanceKey(state, action.Body.Submitter, action.Signature)
	if err != nil {
		return nil, err
	}
	if err := governance.VerifyExecute(action, publicKey); err != nil {
		return nil, fail(Invalid, "invalid execution signature")
	}
	key := action.Body.ProposalID.String()
	record, exists := state.Governance.Proposals[key]
	if !exists {
		return nil, fail(ProposalNotFound, "proposal not found")
	}
	if record.Status == governance.Executed {
		return nil, fail(AlreadyExecuted, "proposal already executed")
	}
	if record.Status != governance.Approved {
		return nil, fail(ProposalClosed, "proposal is not approved")
	}
	var updates []governance.ValidatorUpdate
	switch record.Body.Kind {
	case governance.MembershipChange:
		payload := record.Body.Membership
		target := payload.Target.String()
		if state.Memberships[target] != payload.Expected {
			return nil, fail(Stale, "membership execution precondition changed")
		}
		state.Memberships[target] = payload.Requested
	case governance.ValidatorSetChange:
		payload := record.Body.ValidatorSet
		currentHash, err := state.Governance.ValidatorSetHash()
		if err != nil || !governance.EqualHash(currentHash, payload.ExpectedSetHash) {
			return nil, fail(Stale, "validator-set execution precondition changed")
		}
		if state.Memberships[payload.Validator.Operator.String()] != membership.Active {
			return nil, fail(NotEligible, "validator operator is not ACTIVE")
		}
		validatorKey := governance.ValidatorKey(payload.Validator.PublicKey)
		if payload.Action == governance.AddValidator {
			if _, exists := state.Governance.Validators[validatorKey]; exists {
				return nil, fail(Stale, "validator already exists")
			}
			state.Governance.Validators[validatorKey] = payload.Validator
			updates = []governance.ValidatorUpdate{{PublicKey: append([]byte(nil), payload.Validator.PublicKey...), Power: governance.ValidatorPower}}
		} else {
			current, exists := state.Governance.Validators[validatorKey]
			if !exists || current.Operator.String() != payload.Validator.Operator.String() {
				return nil, fail(Stale, "validator does not match current set")
			}
			if len(state.Governance.Validators)-1 < governance.MinimumValidatorCount {
				return nil, fail(Invalid, "validator removal violates minimum count")
			}
			delete(state.Governance.Validators, validatorKey)
			updates = []governance.ValidatorUpdate{{PublicKey: append([]byte(nil), payload.Validator.PublicKey...), Power: 0}}
		}
	case governance.ProtocolUpgrade, governance.Migration:
		// The approved decision is recorded; software activation/import/export
		// remains an explicit later-phase operational boundary.
	case governance.ResearchAdmission:
		if err := admitResearch(state, record, context.Height); err != nil {
			return nil, err
		}
	case governance.ResourceAdmission:
		if err := admitResource(state, record, context.Height); err != nil {
			return nil, err
		}
	default:
		return nil, fail(Unsupported, "proposal kind has no Phase 6 execution handler")
	}
	record.Status = governance.Executed
	state.Governance.Proposals[key] = record
	return updates, nil
}
