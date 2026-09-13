package governance

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

const (
	Schema protocol.SchemaVersion = 1

	MinimumUniqueVoters     uint64 = 3
	ApprovalNumerator       uint64 = 2
	ApprovalDenominator     uint64 = 3
	VotingPeriodBlocks      uint64 = 10
	MaxElectorateSize              = 1024
	MaxProposalPayloadBytes        = 4096
	MinimumValidatorCount          = 3
	ValidatorPower          int64  = 1
)

type ProposalID struct{ protocol.HashDigest }

func (id ProposalID) String() string {
	if id.Validate() != nil {
		return ""
	}
	return "zion:proposal:sha256:" + hex.EncodeToString(id.Digest)
}

func (id ProposalID) Validate() error { return id.HashDigest.Validate() }

func ParseProposalID(value string) (ProposalID, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 || parts[0] != "zion" || parts[1] != "proposal" || parts[2] != "sha256" || strings.ToLower(parts[3]) != parts[3] {
		return ProposalID{}, fmt.Errorf("invalid proposal ID")
	}
	digest, err := hex.DecodeString(parts[3])
	if err != nil {
		return ProposalID{}, fmt.Errorf("decode proposal ID: %w", err)
	}
	id := ProposalID{HashDigest: protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: digest}}
	return id, id.Validate()
}

type Policy struct {
	SchemaVersion       protocol.SchemaVersion `cbor:"1,keyasint"`
	MinimumParticipants uint64                 `cbor:"2,keyasint"`
	ApprovalNumerator   uint64                 `cbor:"3,keyasint"`
	ApprovalDenominator uint64                 `cbor:"4,keyasint"`
	VotingPeriodBlocks  uint64                 `cbor:"5,keyasint"`
	MaxElectorate       uint64                 `cbor:"6,keyasint"`
	MaxPayloadBytes     uint64                 `cbor:"7,keyasint"`
}

func AlphaPolicy() Policy {
	return Policy{Schema, MinimumUniqueVoters, ApprovalNumerator, ApprovalDenominator,
		VotingPeriodBlocks, MaxElectorateSize, MaxProposalPayloadBytes}
}

func (policy Policy) Validate() error {
	if policy != AlphaPolicy() {
		return fmt.Errorf("unsupported governance policy")
	}
	return nil
}

type ProposalKind string

const (
	MembershipChange   ProposalKind = "MEMBERSHIP_CHANGE"
	ValidatorSetChange ProposalKind = "VALIDATOR_SET_CHANGE"
	ProtocolUpgrade    ProposalKind = "PROTOCOL_UPGRADE"
	Migration          ProposalKind = "MIGRATION"
	ResearchAdmission  ProposalKind = "RESEARCH_ADMISSION"
	ResourceAdmission  ProposalKind = "RESOURCE_ADMISSION"
)

type VoteChoice string

const (
	Yes     VoteChoice = "YES"
	No      VoteChoice = "NO"
	Abstain VoteChoice = "ABSTAIN"
)

func (choice VoteChoice) Validate() error {
	if choice != Yes && choice != No && choice != Abstain {
		return fmt.Errorf("invalid vote choice %q", choice)
	}
	return nil
}

type ProposalStatus string

const (
	Open     ProposalStatus = "OPEN"
	Approved ProposalStatus = "APPROVED"
	Rejected ProposalStatus = "REJECTED"
	Expired  ProposalStatus = "EXPIRED"
	Executed ProposalStatus = "EXECUTED"
)

type MembershipChangePayload struct {
	Target    identity.IdentityID `cbor:"1,keyasint"`
	Expected  membership.Status   `cbor:"2,keyasint"`
	Requested membership.Status   `cbor:"3,keyasint"`
}

func (payload MembershipChangePayload) Validate() error {
	if err := payload.Target.Validate(); err != nil {
		return err
	}
	if err := payload.Expected.Validate(); err != nil {
		return err
	}
	if err := payload.Requested.Validate(); err != nil {
		return err
	}
	allowed := (payload.Expected == membership.Pending && payload.Requested == membership.Active) ||
		(payload.Expected == membership.Active && (payload.Requested == membership.Suspended || payload.Requested == membership.Revoked)) ||
		(payload.Expected == membership.Suspended && (payload.Requested == membership.Active || payload.Requested == membership.Revoked))
	if !allowed {
		return fmt.Errorf("unsupported membership transition %s to %s", payload.Expected, payload.Requested)
	}
	return nil
}

type ValidatorAction string

const (
	AddValidator    ValidatorAction = "ADD"
	RemoveValidator ValidatorAction = "REMOVE"
)

type Validator struct {
	Operator  identity.IdentityID `cbor:"1,keyasint"`
	PublicKey []byte              `cbor:"2,keyasint"`
	Power     int64               `cbor:"3,keyasint"`
}

func (validator Validator) Validate() error {
	if err := validator.Operator.Validate(); err != nil {
		return err
	}
	if len(validator.PublicKey) != 32 || validator.Power != ValidatorPower {
		return fmt.Errorf("invalid equal-power Ed25519 validator")
	}
	return nil
}

func ValidatorKey(publicKey []byte) string { return hex.EncodeToString(publicKey) }

type ValidatorSetChangePayload struct {
	Action          ValidatorAction     `cbor:"1,keyasint"`
	Validator       Validator           `cbor:"2,keyasint"`
	ExpectedSetHash protocol.HashDigest `cbor:"3,keyasint"`
}

func (payload ValidatorSetChangePayload) Validate() error {
	if payload.Action != AddValidator && payload.Action != RemoveValidator {
		return fmt.Errorf("invalid validator action")
	}
	if err := payload.Validator.Validate(); err != nil {
		return err
	}
	return payload.ExpectedSetHash.Validate()
}

type ProtocolUpgradePayload struct {
	TargetVersion       protocol.ProtocolVersion `cbor:"1,keyasint"`
	ActivationHeight    int64                    `cbor:"2,keyasint"`
	MigrationIdentifier string                   `cbor:"3,keyasint"`
}

type MigrationPayload struct {
	TargetNetwork       protocol.NetworkID       `cbor:"1,keyasint"`
	TargetVersion       protocol.ProtocolVersion `cbor:"2,keyasint"`
	MigrationIdentifier string                   `cbor:"3,keyasint"`
}

type ProposalBody struct {
	SchemaVersion protocol.SchemaVersion     `cbor:"1,keyasint"`
	NetworkID     protocol.NetworkID         `cbor:"2,keyasint"`
	Kind          ProposalKind               `cbor:"3,keyasint"`
	Proposer      identity.IdentityID        `cbor:"4,keyasint"`
	CreatedAt     protocol.ProtocolTimestamp `cbor:"5,keyasint"`
	Membership    *MembershipChangePayload   `cbor:"6,keyasint,omitempty"`
	ValidatorSet  *ValidatorSetChangePayload `cbor:"7,keyasint,omitempty"`
	Upgrade       *ProtocolUpgradePayload    `cbor:"8,keyasint,omitempty"`
	Migration     *MigrationPayload          `cbor:"9,keyasint,omitempty"`
	Research      *research.EntryBody        `cbor:"10,keyasint,omitempty"`
	Resource      *resources.EntryBody       `cbor:"11,keyasint,omitempty"`
}

func (body ProposalBody) ID() (ProposalID, error) {
	if err := body.Validate(); err != nil {
		return ProposalID{}, err
	}
	digest, err := protocol.HashCanonical(body)
	return ProposalID{HashDigest: digest}, err
}

func (body ProposalBody) Validate() error {
	if body.SchemaVersion != Schema || body.NetworkID == "" {
		return fmt.Errorf("invalid proposal schema or network")
	}
	if err := body.Proposer.Validate(); err != nil {
		return err
	}
	selected := 0
	if body.Membership != nil {
		selected++
	}
	if body.ValidatorSet != nil {
		selected++
	}
	if body.Upgrade != nil {
		selected++
	}
	if body.Migration != nil {
		selected++
	}
	if body.Research != nil {
		selected++
	}
	if body.Resource != nil {
		selected++
	}
	if selected != 1 {
		return fmt.Errorf("proposal must contain exactly one payload")
	}
	switch body.Kind {
	case MembershipChange:
		if body.Membership == nil {
			return fmt.Errorf("membership payload mismatch")
		}
		if err := body.Membership.Validate(); err != nil {
			return err
		}
	case ValidatorSetChange:
		if body.ValidatorSet == nil {
			return fmt.Errorf("validator payload mismatch")
		}
		if err := body.ValidatorSet.Validate(); err != nil {
			return err
		}
	case ProtocolUpgrade:
		if body.Upgrade == nil || body.Upgrade.TargetVersion == "" || body.Upgrade.ActivationHeight <= 0 || body.Upgrade.MigrationIdentifier == "" {
			return fmt.Errorf("invalid protocol upgrade payload")
		}
	case Migration:
		if body.Migration == nil || body.Migration.TargetNetwork == "" || body.Migration.TargetVersion == "" || body.Migration.MigrationIdentifier == "" {
			return fmt.Errorf("invalid migration payload")
		}
	case ResearchAdmission:
		if body.Research == nil || body.Research.Validate() != nil {
			return fmt.Errorf("invalid research admission payload")
		}
	case ResourceAdmission:
		if body.Resource == nil || body.Resource.Validate() != nil {
			return fmt.Errorf("invalid resource admission payload")
		}
	default:
		return fmt.Errorf("unknown proposal kind")
	}
	payload, err := protocol.CanonicalEncode(body)
	if err != nil || len(payload) > MaxProposalPayloadBytes {
		return fmt.Errorf("proposal payload exceeds limit")
	}
	return nil
}

type ProposalAuthorization struct {
	Body       ProposalBody       `cbor:"1,keyasint"`
	ProposalID ProposalID         `cbor:"2,keyasint"`
	Signature  identity.Signature `cbor:"3,keyasint"`
}

type VoteBody struct {
	SchemaVersion protocol.SchemaVersion `cbor:"1,keyasint"`
	NetworkID     protocol.NetworkID     `cbor:"2,keyasint"`
	ProposalID    ProposalID             `cbor:"3,keyasint"`
	Voter         identity.IdentityID    `cbor:"4,keyasint"`
	Choice        VoteChoice             `cbor:"5,keyasint"`
}

func (body VoteBody) Validate() error {
	if body.SchemaVersion != Schema || body.NetworkID == "" {
		return fmt.Errorf("invalid vote schema or network")
	}
	if err := body.ProposalID.Validate(); err != nil {
		return err
	}
	if err := body.Voter.Validate(); err != nil {
		return err
	}
	return body.Choice.Validate()
}

type VoteAuthorization struct {
	Body      VoteBody           `cbor:"1,keyasint"`
	Signature identity.Signature `cbor:"2,keyasint"`
}

type ActionBody struct {
	SchemaVersion protocol.SchemaVersion `cbor:"1,keyasint"`
	NetworkID     protocol.NetworkID     `cbor:"2,keyasint"`
	ProposalID    ProposalID             `cbor:"3,keyasint"`
	Submitter     identity.IdentityID    `cbor:"4,keyasint"`
}

type ActionAuthorization struct {
	Body      ActionBody         `cbor:"1,keyasint"`
	Signature identity.Signature `cbor:"2,keyasint"`
}

func (body ActionBody) Validate() error {
	if body.SchemaVersion != Schema || body.NetworkID == "" || body.ProposalID.Validate() != nil || body.Submitter.Validate() != nil {
		return fmt.Errorf("invalid governance action body")
	}
	return nil
}

type VoteRecord struct {
	Voter  identity.IdentityID `cbor:"1,keyasint"`
	Choice VoteChoice          `cbor:"2,keyasint"`
	KeyID  identity.KeyID      `cbor:"3,keyasint"`
}

type ProposalRecord struct {
	ID          ProposalID
	Body        ProposalBody
	Status      ProposalStatus
	StartHeight int64
	EndHeight   int64
	Electorate  []identity.IdentityID
	Votes       map[string]VoteRecord
}

type State struct {
	Policy     Policy
	Proposals  map[string]ProposalRecord
	Validators map[string]Validator
}

type ValidatorUpdate struct {
	PublicKey []byte `cbor:"1,keyasint"`
	Power     int64  `cbor:"2,keyasint"`
}

type Tally struct {
	Yes, No, Abstain, Participants uint64
}

func (record ProposalRecord) Tally() Tally {
	var tally Tally
	for _, vote := range record.Votes {
		tally.Participants++
		switch vote.Choice {
		case Yes:
			tally.Yes++
		case No:
			tally.No++
		case Abstain:
			tally.Abstain++
		}
	}
	return tally
}

func (policy Policy) Decision(tally Tally) ProposalStatus {
	if tally.Participants < policy.MinimumParticipants {
		return Expired
	}
	counted := tally.Yes + tally.No
	if counted != 0 && tally.Yes*policy.ApprovalDenominator > counted*policy.ApprovalNumerator {
		return Approved
	}
	return Rejected
}

func NewState(validators []Validator) (State, error) {
	state := State{Policy: AlphaPolicy(), Proposals: make(map[string]ProposalRecord), Validators: make(map[string]Validator)}
	for _, validator := range validators {
		if err := validator.Validate(); err != nil {
			return State{}, err
		}
		key := ValidatorKey(validator.PublicKey)
		if _, exists := state.Validators[key]; exists {
			return State{}, fmt.Errorf("duplicate validator")
		}
		validator = CloneValidator(validator)
		state.Validators[key] = validator
	}
	if len(state.Validators) < MinimumValidatorCount {
		return State{}, fmt.Errorf("too few validators")
	}
	return state, nil
}

func (state State) ValidatorSetHash() (protocol.HashDigest, error) {
	keys := make([]string, 0, len(state.Validators))
	for key := range state.Validators {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	validators := make([]Validator, 0, len(keys))
	for _, key := range keys {
		validators = append(validators, state.Validators[key])
	}
	return protocol.HashCanonical(validators)
}

func (state State) Clone() State {
	clone := State{Policy: state.Policy, Proposals: make(map[string]ProposalRecord, len(state.Proposals)), Validators: make(map[string]Validator, len(state.Validators))}
	for key, validator := range state.Validators {
		validator = CloneValidator(validator)
		clone.Validators[key] = validator
	}
	for key, record := range state.Proposals {
		record.ID = cloneProposalID(record.ID)
		record.Body = CloneProposalBody(record.Body)
		record.Electorate = cloneIdentityIDs(record.Electorate)
		record.Votes = make(map[string]VoteRecord, len(record.Votes))
		for voter, vote := range state.Proposals[key].Votes {
			record.Votes[voter] = cloneVoteRecord(vote)
		}
		clone.Proposals[key] = record
	}
	return clone
}

func CloneProposalBody(body ProposalBody) ProposalBody {
	clone := body
	clone.Proposer = cloneIdentityID(body.Proposer)
	if body.Membership != nil {
		payload := *body.Membership
		payload.Target = cloneIdentityID(payload.Target)
		clone.Membership = &payload
	}
	if body.ValidatorSet != nil {
		payload := *body.ValidatorSet
		payload.Validator = CloneValidator(payload.Validator)
		payload.ExpectedSetHash = cloneHash(payload.ExpectedSetHash)
		clone.ValidatorSet = &payload
	}
	if body.Upgrade != nil {
		payload := *body.Upgrade
		clone.Upgrade = &payload
	}
	if body.Migration != nil {
		payload := *body.Migration
		clone.Migration = &payload
	}
	if body.Research != nil {
		payload := research.CloneBody(*body.Research)
		clone.Research = &payload
	}
	if body.Resource != nil {
		payload := resources.CloneBody(*body.Resource)
		clone.Resource = &payload
	}
	return clone
}

func CloneValidator(validator Validator) Validator {
	validator.Operator = cloneIdentityID(validator.Operator)
	validator.PublicKey = append([]byte(nil), validator.PublicKey...)
	return validator
}

func cloneIdentityID(id identity.IdentityID) identity.IdentityID {
	return identity.IdentityID{HashDigest: cloneHash(id.HashDigest)}
}

func cloneIdentityIDs(ids []identity.IdentityID) []identity.IdentityID {
	result := make([]identity.IdentityID, len(ids))
	for i, id := range ids {
		result[i] = cloneIdentityID(id)
	}
	return result
}

func cloneProposalID(id ProposalID) ProposalID {
	return ProposalID{HashDigest: cloneHash(id.HashDigest)}
}

func cloneHash(hash protocol.HashDigest) protocol.HashDigest {
	return protocol.HashDigest{Algorithm: hash.Algorithm, Digest: append([]byte(nil), hash.Digest...)}
}

func cloneVoteRecord(vote VoteRecord) VoteRecord {
	vote.Voter = cloneIdentityID(vote.Voter)
	vote.KeyID = identity.KeyID{HashDigest: cloneHash(vote.KeyID.HashDigest)}
	return vote
}

type Snapshot struct {
	Policy     Policy             `cbor:"1,keyasint"`
	Proposals  []ProposalSnapshot `cbor:"2,keyasint"`
	Validators []Validator        `cbor:"3,keyasint"`
}

type ProposalSnapshot struct {
	ID          ProposalID            `cbor:"1,keyasint"`
	Body        ProposalBody          `cbor:"2,keyasint"`
	Status      ProposalStatus        `cbor:"3,keyasint"`
	StartHeight int64                 `cbor:"4,keyasint"`
	EndHeight   int64                 `cbor:"5,keyasint"`
	Electorate  []identity.IdentityID `cbor:"6,keyasint"`
	Votes       []VoteRecord          `cbor:"7,keyasint"`
}

func (state State) Snapshot() (Snapshot, error) {
	if err := state.Policy.Validate(); err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{Policy: state.Policy, Proposals: make([]ProposalSnapshot, 0, len(state.Proposals)), Validators: make([]Validator, 0, len(state.Validators))}
	proposalKeys := make([]string, 0, len(state.Proposals))
	for key := range state.Proposals {
		proposalKeys = append(proposalKeys, key)
	}
	sort.Strings(proposalKeys)
	for _, key := range proposalKeys {
		record := state.Proposals[key]
		if record.ID.String() != key {
			return Snapshot{}, fmt.Errorf("proposal key mismatch")
		}
		derivedID, err := record.Body.ID()
		if err != nil || derivedID.String() != key {
			return Snapshot{}, fmt.Errorf("invalid stored proposal body")
		}
		if record.Status != Open && record.Status != Approved && record.Status != Rejected && record.Status != Expired && record.Status != Executed {
			return Snapshot{}, fmt.Errorf("invalid proposal status")
		}
		if record.StartHeight <= 0 || record.EndHeight <= record.StartHeight || len(record.Electorate) > int(state.Policy.MaxElectorate) || len(record.Votes) > len(record.Electorate) {
			return Snapshot{}, fmt.Errorf("invalid proposal bounds")
		}
		electorate := cloneIdentityIDs(record.Electorate)
		sort.Slice(electorate, func(i, j int) bool { return electorate[i].String() < electorate[j].String() })
		for i, memberID := range electorate {
			if memberID.Validate() != nil || (i > 0 && electorate[i-1].String() == memberID.String()) {
				return Snapshot{}, fmt.Errorf("invalid proposal electorate")
			}
		}
		voteKeys := make([]string, 0, len(record.Votes))
		for voter := range record.Votes {
			voteKeys = append(voteKeys, voter)
		}
		sort.Strings(voteKeys)
		votes := make([]VoteRecord, 0, len(voteKeys))
		for _, voter := range voteKeys {
			vote := record.Votes[voter]
			if vote.Voter.String() != voter || vote.Choice.Validate() != nil || !ContainsIdentity(electorate, vote.Voter) || vote.KeyID.Validate() != nil {
				return Snapshot{}, fmt.Errorf("invalid stored vote")
			}
			votes = append(votes, cloneVoteRecord(vote))
		}
		result.Proposals = append(result.Proposals, ProposalSnapshot{cloneProposalID(record.ID), CloneProposalBody(record.Body), record.Status, record.StartHeight, record.EndHeight, electorate, votes})
	}
	if len(state.Validators) < MinimumValidatorCount {
		return Snapshot{}, fmt.Errorf("validator set below minimum")
	}
	validatorKeys := make([]string, 0, len(state.Validators))
	for key := range state.Validators {
		validatorKeys = append(validatorKeys, key)
	}
	sort.Strings(validatorKeys)
	for _, key := range validatorKeys {
		validator := state.Validators[key]
		if err := validator.Validate(); err != nil || ValidatorKey(validator.PublicKey) != key {
			return Snapshot{}, fmt.Errorf("invalid validator state")
		}
		validator = CloneValidator(validator)
		result.Validators = append(result.Validators, validator)
	}
	return result, nil
}

func ContainsIdentity(ids []identity.IdentityID, target identity.IdentityID) bool {
	for _, id := range ids {
		if id.String() == target.String() {
			return true
		}
	}
	return false
}

func EqualHash(left, right protocol.HashDigest) bool {
	return left.Algorithm == right.Algorithm && bytes.Equal(left.Digest, right.Digest)
}
