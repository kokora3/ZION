// Package chain implements the deterministic, ordered ZION state transition
// function. It deliberately has no consensus, networking, or persistence.
package chain

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

type TransactionType string

const (
	IdentityCreate     TransactionType = "IdentityCreate"
	KeyRotation        TransactionType = "KeyRotation"
	MembershipChange   TransactionType = "MembershipChange"
	GovernanceProposal TransactionType = "GovernanceProposal"
	GovernanceVote     TransactionType = "GovernanceVote"
	GovernanceFinalize TransactionType = "GovernanceFinalize"
	GovernanceExecute  TransactionType = "GovernanceExecute"
	ValidatorSetChange TransactionType = "ValidatorSetChange"

	TransactionSchema protocol.SchemaVersion = 1
	StateSchemaV1     protocol.SchemaVersion = 1
	StateSchemaV2     protocol.SchemaVersion = 2
	StateSchemaV3     protocol.SchemaVersion = 3
	StateSchema                              = StateSchemaV1

	// MaxTransactionBytes is a protocol constant, not a node configuration
	// value. Every validator applies it to the canonical transaction bytes.
	MaxTransactionBytes = 64 * 1024
)

// TxID is the SHA-256 digest of the complete canonical transaction envelope.
// Transaction has no TxID field, so the identifier cannot include itself.
type TxID struct{ protocol.HashDigest }

func (id TxID) String() string {
	if id.Validate() != nil {
		return ""
	}
	return "zion:tx:sha256:" + hex.EncodeToString(id.Digest)
}

func (id TxID) Validate() error { return id.HashDigest.Validate() }

func ParseTxID(s string) (TxID, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 4 || parts[0] != "zion" || parts[1] != "tx" || parts[2] != "sha256" || strings.ToLower(parts[3]) != parts[3] {
		return TxID{}, fmt.Errorf("invalid transaction ID")
	}
	digest, err := hex.DecodeString(parts[3])
	if err != nil {
		return TxID{}, fmt.Errorf("decode transaction ID: %w", err)
	}
	id := TxID{HashDigest: protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: digest}}
	return id, id.Validate()
}

// Transaction is the canonical chain envelope. The selected payload contains
// the Phase 3 signatures/proofs; exactly one matching payload is accepted.
type Transaction struct {
	SchemaVersion      protocol.SchemaVersion            `cbor:"1,keyasint"`
	NetworkID          protocol.NetworkID                `cbor:"2,keyasint"`
	Type               TransactionType                   `cbor:"3,keyasint"`
	IdentityCreate     *identity.IdentityGenesisProof    `cbor:"4,keyasint,omitempty"`
	KeyRotation        *identity.RotationProof           `cbor:"5,keyasint,omitempty"`
	GovernanceProposal *governance.ProposalAuthorization `cbor:"6,keyasint,omitempty"`
	GovernanceVote     *governance.VoteAuthorization     `cbor:"7,keyasint,omitempty"`
	GovernanceFinalize *governance.ActionAuthorization   `cbor:"8,keyasint,omitempty"`
	GovernanceExecute  *governance.ActionAuthorization   `cbor:"9,keyasint,omitempty"`
}

func (tx Transaction) CanonicalBytes() ([]byte, error) {
	return protocol.CanonicalEncode(tx)
}

func (tx Transaction) ID() (TxID, error) {
	canonical, err := tx.CanonicalBytes()
	if err != nil {
		return TxID{}, err
	}
	return TxID{HashDigest: protocol.HashBytes(canonical)}, nil
}

type Code string

const (
	OK                    Code = "OK"
	WrongNetwork          Code = "ERR_WRONG_NETWORK"
	Unsupported           Code = "ERR_UNSUPPORTED"
	Invalid               Code = "ERR_INVALID"
	TooLarge              Code = "ERR_TRANSACTION_TOO_LARGE"
	Exists                Code = "ERR_IDENTITY_EXISTS"
	NotFound              Code = "ERR_IDENTITY_NOT_FOUND"
	KeyNotActive          Code = "ERR_KEY_NOT_ACTIVE"
	Sequence              Code = "ERR_ROTATION_SEQUENCE"
	GovernanceUnavailable Code = "ERR_GOVERNANCE_UNAVAILABLE"
	NotEligible           Code = "ERR_NOT_ELIGIBLE"
	ProposalExists        Code = "ERR_PROPOSAL_EXISTS"
	ProposalNotFound      Code = "ERR_PROPOSAL_NOT_FOUND"
	ProposalClosed        Code = "ERR_PROPOSAL_CLOSED"
	DuplicateVote         Code = "ERR_DUPLICATE_VOTE"
	NotReady              Code = "ERR_PROPOSAL_NOT_READY"
	Stale                 Code = "ERR_STALE_EXECUTION"
	AlreadyExecuted       Code = "ERR_ALREADY_EXECUTED"
	RegistryExists        Code = "ERR_REGISTRY_EXISTS"
)

// Receipt contains only deterministic values derived from the input state and
// transaction. Local time, randomness, process details, and error strings are
// deliberately excluded.
type Receipt struct {
	TxID             TxID                         `cbor:"1,keyasint"`
	Type             TransactionType              `cbor:"2,keyasint"`
	Code             Code                         `cbor:"3,keyasint"`
	StateHash        StateHash                    `cbor:"4,keyasint"`
	ValidatorUpdates []governance.ValidatorUpdate `cbor:"5,keyasint,omitempty"`
}

type StateHash struct{ protocol.HashDigest }

func (hash StateHash) String() string {
	if hash.Validate() != nil {
		return ""
	}
	return "zion:state:sha256:" + hex.EncodeToString(hash.Digest)
}

func (hash StateHash) Validate() error { return hash.HashDigest.Validate() }

// StoredIdentity contains public protocol material only. Private keys and
// Ed25519 seeds are never accepted by, or retained in, State.
type StoredIdentity struct {
	ID       identity.IdentityID
	Keys     map[string]identity.PublicKey
	Active   map[string]bool
	Sequence uint64
	Revoked  bool
}

// State is the in-memory canonical chain state. Its maps are converted to
// explicitly sorted slices before hashing or encoding.
type State struct {
	SchemaVersion   protocol.SchemaVersion
	NetworkID       protocol.NetworkID
	ProtocolVersion protocol.ProtocolVersion
	Identities      map[string]StoredIdentity
	Memberships     map[string]membership.Status
	Governance      *governance.State
	Research        map[string]StoredResearch
	Resources       map[string]StoredResource
}

func Genesis(network protocol.NetworkID) State {
	return State{
		SchemaVersion:   StateSchema,
		NetworkID:       network,
		ProtocolVersion: protocol.CurrentProtocolVersion,
		Identities:      make(map[string]StoredIdentity),
		Memberships:     make(map[string]membership.Status),
	}
}

// Snapshot is the canonical, allocation-independent representation of State.
type Snapshot struct {
	SchemaVersion   protocol.SchemaVersion   `cbor:"1,keyasint"`
	NetworkID       protocol.NetworkID       `cbor:"2,keyasint"`
	ProtocolVersion protocol.ProtocolVersion `cbor:"3,keyasint"`
	Identities      []SnapshotIdentity       `cbor:"4,keyasint"`
	Memberships     []SnapshotMember         `cbor:"5,keyasint"`
	Governance      *governance.Snapshot     `cbor:"6,keyasint,omitempty"`
	Research        []SnapshotResearch       `cbor:"7,keyasint,omitempty"`
	Resources       []SnapshotResource       `cbor:"8,keyasint,omitempty"`
}

type SnapshotIdentity struct {
	ID       identity.IdentityID `cbor:"1,keyasint"`
	Keys     []SnapshotKey       `cbor:"2,keyasint"`
	Sequence uint64              `cbor:"3,keyasint"`
	Revoked  bool                `cbor:"4,keyasint"`
}

type SnapshotKey struct {
	ID     identity.KeyID     `cbor:"1,keyasint"`
	Public identity.PublicKey `cbor:"2,keyasint"`
	Active bool               `cbor:"3,keyasint"`
}

type SnapshotMember struct {
	ID     identity.IdentityID `cbor:"1,keyasint"`
	Status membership.Status   `cbor:"2,keyasint"`
}

func (s State) Snapshot() (Snapshot, error) {
	if s.SchemaVersion != StateSchemaV1 && s.SchemaVersion != StateSchemaV2 && s.SchemaVersion != StateSchemaV3 {
		return Snapshot{}, fmt.Errorf("unsupported state schema %d", s.SchemaVersion)
	}
	if s.SchemaVersion == StateSchemaV1 && s.Governance != nil {
		return Snapshot{}, fmt.Errorf("governance state requires state schema v2")
	}
	if (s.SchemaVersion == StateSchemaV2 || s.SchemaVersion == StateSchemaV3) && s.Governance == nil {
		return Snapshot{}, fmt.Errorf("state schema v2 requires governance state")
	}
	if s.SchemaVersion != StateSchemaV3 && (s.Research != nil || s.Resources != nil) {
		return Snapshot{}, fmt.Errorf("registry state requires state schema v3")
	}
	if s.SchemaVersion == StateSchemaV3 && (s.Research == nil || s.Resources == nil) {
		return Snapshot{}, fmt.Errorf("state schema v3 requires registry state")
	}
	if s.NetworkID == "" {
		return Snapshot{}, fmt.Errorf("empty state network")
	}
	if s.ProtocolVersion != protocol.CurrentProtocolVersion {
		return Snapshot{}, fmt.Errorf("unsupported protocol version %q", s.ProtocolVersion)
	}
	result := Snapshot{
		SchemaVersion:   s.SchemaVersion,
		NetworkID:       s.NetworkID,
		ProtocolVersion: s.ProtocolVersion,
		Identities:      make([]SnapshotIdentity, 0, len(s.Identities)),
		Memberships:     make([]SnapshotMember, 0, len(s.Memberships)),
	}

	identityIDs := make([]string, 0, len(s.Identities))
	for id := range s.Identities {
		identityIDs = append(identityIDs, id)
	}
	sort.Strings(identityIDs)
	for _, idString := range identityIDs {
		stored := s.Identities[idString]
		if err := stored.ID.Validate(); err != nil || stored.ID.String() != idString {
			return Snapshot{}, fmt.Errorf("invalid stored identity %q", idString)
		}
		item := SnapshotIdentity{
			ID:       cloneIdentityID(stored.ID),
			Keys:     make([]SnapshotKey, 0, len(stored.Keys)),
			Sequence: stored.Sequence,
			Revoked:  stored.Revoked,
		}

		keyIDs := make([]string, 0, len(stored.Keys))
		for keyID := range stored.Keys {
			keyIDs = append(keyIDs, keyID)
		}
		sort.Strings(keyIDs)
		for _, keyIDString := range keyIDs {
			publicKey := stored.Keys[keyIDString]
			keyID, err := identity.DeriveKeyID(publicKey)
			if err != nil || keyID.String() != keyIDString {
				return Snapshot{}, fmt.Errorf("invalid stored key %q", keyIDString)
			}
			active, ok := stored.Active[keyIDString]
			if !ok {
				return Snapshot{}, fmt.Errorf("missing key status %q", keyIDString)
			}
			item.Keys = append(item.Keys, SnapshotKey{
				ID:     cloneKeyID(keyID),
				Public: clonePublicKey(publicKey),
				Active: active,
			})
		}
		if len(stored.Active) != len(stored.Keys) {
			return Snapshot{}, fmt.Errorf("key status set differs from key set for %q", idString)
		}
		result.Identities = append(result.Identities, item)
	}

	membershipIDs := make([]string, 0, len(s.Memberships))
	for id := range s.Memberships {
		membershipIDs = append(membershipIDs, id)
	}
	sort.Strings(membershipIDs)
	for _, idString := range membershipIDs {
		if _, ok := s.Identities[idString]; !ok {
			return Snapshot{}, fmt.Errorf("membership without identity %q", idString)
		}
		id, err := identity.ParseIdentityID(idString)
		if err != nil {
			return Snapshot{}, fmt.Errorf("invalid membership identity %q", idString)
		}
		status := s.Memberships[idString]
		if err := status.Validate(); err != nil {
			return Snapshot{}, err
		}
		result.Memberships = append(result.Memberships, SnapshotMember{
			ID:     cloneIdentityID(id),
			Status: status,
		})
	}
	if len(s.Memberships) != len(s.Identities) {
		return Snapshot{}, fmt.Errorf("membership set differs from identity set")
	}
	if s.Governance != nil {
		governanceSnapshot, err := s.Governance.Snapshot()
		if err != nil {
			return Snapshot{}, fmt.Errorf("governance snapshot: %w", err)
		}
		for _, validator := range governanceSnapshot.Validators {
			if _, exists := s.Identities[validator.Operator.String()]; !exists {
				return Snapshot{}, fmt.Errorf("validator operator identity not found")
			}
		}
		result.Governance = &governanceSnapshot
	}
	if s.SchemaVersion == StateSchemaV3 {
		if err := snapshotRegistries(s, &result); err != nil {
			return Snapshot{}, err
		}
	}

	return result, nil
}

func (s State) CanonicalBytes() ([]byte, error) {
	snapshot, err := s.Snapshot()
	if err != nil {
		return nil, err
	}
	return protocol.CanonicalEncode(snapshot)
}

func (s State) Hash() (StateHash, error) {
	canonical, err := s.CanonicalBytes()
	if err != nil {
		return StateHash{}, err
	}
	return StateHash{HashDigest: protocol.HashBytes(canonical)}, nil
}

func cloneState(s State) State {
	cloned := State{
		SchemaVersion:   s.SchemaVersion,
		NetworkID:       s.NetworkID,
		ProtocolVersion: s.ProtocolVersion,
		Identities:      make(map[string]StoredIdentity, len(s.Identities)),
		Memberships:     make(map[string]membership.Status, len(s.Memberships)),
	}
	if s.Research != nil {
		cloned.Research = make(map[string]StoredResearch, len(s.Research))
		for key, entry := range s.Research {
			cloned.Research[key] = cloneStoredResearch(entry)
		}
	}
	if s.Resources != nil {
		cloned.Resources = make(map[string]StoredResource, len(s.Resources))
		for key, entry := range s.Resources {
			cloned.Resources[key] = cloneStoredResource(entry)
		}
	}
	if s.Governance != nil {
		governanceCopy := s.Governance.Clone()
		cloned.Governance = &governanceCopy
	}
	for id, stored := range s.Identities {
		copyOfStored := StoredIdentity{
			ID:       cloneIdentityID(stored.ID),
			Keys:     make(map[string]identity.PublicKey, len(stored.Keys)),
			Active:   make(map[string]bool, len(stored.Active)),
			Sequence: stored.Sequence,
			Revoked:  stored.Revoked,
		}
		for keyID, publicKey := range stored.Keys {
			copyOfStored.Keys[keyID] = clonePublicKey(publicKey)
		}
		for keyID, active := range stored.Active {
			copyOfStored.Active[keyID] = active
		}
		cloned.Identities[id] = copyOfStored
	}
	for id, status := range s.Memberships {
		cloned.Memberships[id] = status
	}
	return cloned
}

func clonePublicKey(publicKey identity.PublicKey) identity.PublicKey {
	return identity.PublicKey{
		Algorithm: publicKey.Algorithm,
		Key:       append([]byte(nil), publicKey.Key...),
	}
}

func cloneIdentityID(id identity.IdentityID) identity.IdentityID {
	return identity.IdentityID{HashDigest: protocol.HashDigest{
		Algorithm: id.Algorithm,
		Digest:    append([]byte(nil), id.Digest...),
	}}
}

func cloneKeyID(id identity.KeyID) identity.KeyID {
	return identity.KeyID{HashDigest: protocol.HashDigest{
		Algorithm: id.Algorithm,
		Digest:    append([]byte(nil), id.Digest...),
	}}
}

// Apply evaluates exactly one transaction. It never reorders transactions and
// publishes the copy-on-write state only after every check succeeds.
func Apply(s State, tx Transaction) (State, Receipt, error) {
	return ApplyWithContext(s, tx, ExecutionContext{})
}

// ExecutionContext contains only consensus-derived inputs. Height zero is
// retained for backward-compatible Phase 4 transactions and is rejected by
// governance transactions.
type ExecutionContext struct {
	Height int64
}

func ApplyWithContext(s State, tx Transaction, context ExecutionContext) (State, Receipt, error) {
	canonical, err := tx.CanonicalBytes()
	if err != nil {
		return reject(s, TxID{}, tx.Type, Invalid, "canonical transaction", err)
	}
	txID := TxID{HashDigest: protocol.HashBytes(canonical)}
	if len(canonical) > MaxTransactionBytes {
		return reject(s, txID, tx.Type, TooLarge, "transaction exceeds protocol size limit", nil)
	}
	if tx.SchemaVersion != TransactionSchema {
		return reject(s, txID, tx.Type, Unsupported, "unsupported transaction schema", nil)
	}
	if tx.NetworkID != s.NetworkID {
		return reject(s, txID, tx.Type, WrongNetwork, "wrong transaction network", nil)
	}

	next := cloneState(s)
	switch tx.Type {
	case IdentityCreate:
		if tx.IdentityCreate == nil || transactionPayloadCount(tx) != 1 {
			return reject(s, txID, tx.Type, Invalid, "invalid identity-create payload", nil)
		}
		proof := *tx.IdentityCreate
		if err := identity.VerifyIdentityGenesis(proof); err != nil {
			return reject(s, txID, tx.Type, Invalid, "invalid identity proof", err)
		}
		identityID := proof.IdentityID.String()
		if _, exists := next.Identities[identityID]; exists {
			return reject(s, txID, tx.Type, Exists, "identity already exists", nil)
		}
		keyID := proof.InitialKeyID.String()
		next.Identities[identityID] = StoredIdentity{
			ID:       cloneIdentityID(proof.IdentityID),
			Keys:     map[string]identity.PublicKey{keyID: clonePublicKey(proof.Body.InitialPublicKey)},
			Active:   map[string]bool{keyID: true},
			Sequence: 0,
			Revoked:  false,
		}
		next.Memberships[identityID] = membership.Pending

	case KeyRotation:
		if tx.KeyRotation == nil || transactionPayloadCount(tx) != 1 {
			return reject(s, txID, tx.Type, Invalid, "invalid key-rotation payload", nil)
		}
		proof := *tx.KeyRotation
		if proof.Request.NetworkID != tx.NetworkID {
			return reject(s, txID, tx.Type, WrongNetwork, "wrong rotation network", nil)
		}
		identityID := proof.Request.IdentityID.String()
		stored, exists := next.Identities[identityID]
		if !exists {
			return reject(s, txID, tx.Type, NotFound, "identity not found", nil)
		}
		if proof.Request.Sequence != stored.Sequence+1 {
			return reject(s, txID, tx.Type, Sequence, "wrong rotation sequence", nil)
		}
		oldKeyID := proof.Request.OldKeyID.String()
		oldPublicKey, exists := stored.Keys[oldKeyID]
		if !exists || !stored.Active[oldKeyID] {
			return reject(s, txID, tx.Type, KeyNotActive, "old key is not active", nil)
		}
		if err := identity.VerifyRotation(proof, oldPublicKey); err != nil {
			return reject(s, txID, tx.Type, Invalid, "invalid rotation proof", err)
		}
		newKeyID, err := identity.DeriveKeyID(proof.Request.NewPublicKey)
		if err != nil {
			return reject(s, txID, tx.Type, Invalid, "invalid new key", err)
		}
		if _, exists := stored.Keys[newKeyID.String()]; exists {
			return reject(s, txID, tx.Type, Invalid, "new key already exists", nil)
		}
		stored.Active[oldKeyID] = false
		stored.Keys[newKeyID.String()] = clonePublicKey(proof.Request.NewPublicKey)
		stored.Active[newKeyID.String()] = true
		stored.Sequence = proof.Request.Sequence
		next.Identities[identityID] = stored

	case MembershipChange:
		return reject(s, txID, tx.Type, Unsupported, "membership governance authorization is deferred", nil)

	case GovernanceProposal, GovernanceVote, GovernanceFinalize, GovernanceExecute:
		return applyGovernance(s, next, tx, txID, context)

	case ValidatorSetChange:
		return reject(s, txID, tx.Type, Unsupported, "direct validator-set changes require governance", nil)

	default:
		return reject(s, txID, tx.Type, Unsupported, "unsupported transaction type", nil)
	}

	stateHash, err := next.Hash()
	if err != nil {
		return reject(s, txID, tx.Type, Invalid, "invalid resulting state", err)
	}
	return next, Receipt{TxID: txID, Type: tx.Type, Code: OK, StateHash: stateHash}, nil
}

func transactionPayloadCount(tx Transaction) int {
	count := 0
	if tx.IdentityCreate != nil {
		count++
	}
	if tx.KeyRotation != nil {
		count++
	}
	if tx.GovernanceProposal != nil {
		count++
	}
	if tx.GovernanceVote != nil {
		count++
	}
	if tx.GovernanceFinalize != nil {
		count++
	}
	if tx.GovernanceExecute != nil {
		count++
	}
	return count
}

func reject(s State, txID TxID, txType TransactionType, code Code, message string, cause error) (State, Receipt, error) {
	stateHash, hashErr := s.Hash()
	receipt := Receipt{TxID: txID, Type: txType, Code: code, StateHash: stateHash}
	if hashErr != nil {
		return s, receipt, fmt.Errorf("%s: hash input state: %w", message, hashErr)
	}
	if cause != nil {
		return s, receipt, fmt.Errorf("%s: %w", message, cause)
	}
	return s, receipt, fmt.Errorf("%s", message)
}
