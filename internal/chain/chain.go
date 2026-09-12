// Package chain implements the deterministic, ordered ZION state transition
// function. It deliberately has no consensus, networking, or persistence.
package chain

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

type TransactionType string

const (
	IdentityCreate   TransactionType = "IdentityCreate"
	KeyRotation      TransactionType = "KeyRotation"
	MembershipChange TransactionType = "MembershipChange"

	TransactionSchema protocol.SchemaVersion = 1
	StateSchema       protocol.SchemaVersion = 1

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
	SchemaVersion  protocol.SchemaVersion         `cbor:"1,keyasint"`
	NetworkID      protocol.NetworkID             `cbor:"2,keyasint"`
	Type           TransactionType                `cbor:"3,keyasint"`
	IdentityCreate *identity.IdentityGenesisProof `cbor:"4,keyasint,omitempty"`
	KeyRotation    *identity.RotationProof        `cbor:"5,keyasint,omitempty"`
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
	OK           Code = "OK"
	WrongNetwork Code = "ERR_WRONG_NETWORK"
	Unsupported  Code = "ERR_UNSUPPORTED"
	Invalid      Code = "ERR_INVALID"
	TooLarge     Code = "ERR_TRANSACTION_TOO_LARGE"
	Exists       Code = "ERR_IDENTITY_EXISTS"
	NotFound     Code = "ERR_IDENTITY_NOT_FOUND"
	KeyNotActive Code = "ERR_KEY_NOT_ACTIVE"
	Sequence     Code = "ERR_ROTATION_SEQUENCE"
)

// Receipt contains only deterministic values derived from the input state and
// transaction. Local time, randomness, process details, and error strings are
// deliberately excluded.
type Receipt struct {
	TxID      TxID            `cbor:"1,keyasint"`
	Type      TransactionType `cbor:"2,keyasint"`
	Code      Code            `cbor:"3,keyasint"`
	StateHash StateHash       `cbor:"4,keyasint"`
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
	if s.SchemaVersion != StateSchema {
		return Snapshot{}, fmt.Errorf("unsupported state schema %d", s.SchemaVersion)
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
		if tx.IdentityCreate == nil || tx.KeyRotation != nil {
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
		if tx.KeyRotation == nil || tx.IdentityCreate != nil {
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

	default:
		return reject(s, txID, tx.Type, Unsupported, "unsupported transaction type", nil)
	}

	stateHash, err := next.Hash()
	if err != nil {
		return reject(s, txID, tx.Type, Invalid, "invalid resulting state", err)
	}
	return next, Receipt{TxID: txID, Type: tx.Type, Code: OK, StateHash: stateHash}, nil
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
