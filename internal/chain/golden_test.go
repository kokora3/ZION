package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

const publicFixtureNotice = "TEST ONLY - PUBLIC FIXTURE - NOT SECRET - NEVER USE IN PRODUCTION"

type identityCreateGolden struct {
	CanonicalTransactionCBORHex string `json:"canonical_transaction_cbor_hex"`
	CreatedAt                   int64  `json:"created_at"`
	ExpectedResultCode          string `json:"expected_result_code"`
	ExpectedStateHash           string `json:"expected_state_hash"`
	FixtureNotice               string `json:"fixture_notice"`
	IdentityID                  string `json:"identity_id"`
	InitialKeyID                string `json:"initial_key_id"`
	InitialPublicKeyAlgorithm   string `json:"initial_public_key_algorithm"`
	InitialPublicKeyHex         string `json:"initial_public_key_hex"`
	NetworkID                   string `json:"network_id"`
	SchemaVersion               uint16 `json:"schema_version"`
	SignatureAlgorithm          string `json:"signature_algorithm"`
	SignatureHex                string `json:"signature_hex"`
	SignatureKeyID              string `json:"signature_key_id"`
	TransactionType             string `json:"transaction_type"`
	TxID                        string `json:"tx_id"`
}

type keyRotationGolden struct {
	CanonicalTransactionCBORHex  string `json:"canonical_transaction_cbor_hex"`
	CreatedAt                    int64  `json:"created_at"`
	ExpectedResultCode           string `json:"expected_result_code"`
	ExpectedStateHash            string `json:"expected_state_hash"`
	FixtureNotice                string `json:"fixture_notice"`
	IdentityID                   string `json:"identity_id"`
	NetworkID                    string `json:"network_id"`
	NewKeyID                     string `json:"new_key_id"`
	NewKeyProofAlgorithm         string `json:"new_key_proof_algorithm"`
	NewKeyProofKeyID             string `json:"new_key_proof_key_id"`
	NewKeyProofSignatureHex      string `json:"new_key_proof_signature_hex"`
	NewPublicKeyAlgorithm        string `json:"new_public_key_algorithm"`
	NewPublicKeyHex              string `json:"new_public_key_hex"`
	OldAuthorizationAlgorithm    string `json:"old_authorization_algorithm"`
	OldAuthorizationKeyID        string `json:"old_authorization_key_id"`
	OldAuthorizationSignatureHex string `json:"old_authorization_signature_hex"`
	OldKeyID                     string `json:"old_key_id"`
	RotationSchemaVersion        uint16 `json:"rotation_schema_version"`
	RotationSequence             uint64 `json:"rotation_sequence"`
	SchemaVersion                uint16 `json:"schema_version"`
	TransactionType              string `json:"transaction_type"`
	TxID                         string `json:"tx_id"`
}

type stateGolden struct {
	CanonicalSnapshotCBORHex string   `json:"canonical_snapshot_cbor_hex"`
	FixtureNotice            string   `json:"fixture_notice"`
	IdentityID               string   `json:"identity_id"`
	MembershipStatus         string   `json:"membership_status"`
	NetworkID                string   `json:"network_id"`
	ProtocolVersion          string   `json:"protocol_version"`
	RotationSequence         uint64   `json:"rotation_sequence"`
	Scenario                 []string `json:"scenario"`
	SchemaVersion            uint16   `json:"schema_version"`
	SHA256DigestHex          string   `json:"sha256_digest_hex"`
	StateHash                string   `json:"state_hash"`
}

func loadGolden[T any](t *testing.T, name string) T {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var fixture T
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func decodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func parseIdentityID(t *testing.T, value string) identity.IdentityID {
	t.Helper()
	id, err := identity.ParseIdentityID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func parseKeyID(t *testing.T, value string) identity.KeyID {
	t.Helper()
	id, err := identity.ParseKeyID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func transactionFromIdentityFixture(t *testing.T, fixture identityCreateGolden) Transaction {
	t.Helper()
	if fixture.FixtureNotice != publicFixtureNotice {
		t.Fatal("identity fixture safety notice changed")
	}
	proof := identity.IdentityGenesisProof{
		Body: identity.IdentityGenesisBody{
			SchemaVersion:    protocol.SchemaVersion(fixture.SchemaVersion),
			InitialPublicKey: identity.PublicKey{Algorithm: identity.SigningAlgorithm(fixture.InitialPublicKeyAlgorithm), Key: decodeHex(t, fixture.InitialPublicKeyHex)},
			CreatedAt:        protocol.ProtocolTimestamp(fixture.CreatedAt),
		},
		IdentityID:   parseIdentityID(t, fixture.IdentityID),
		InitialKeyID: parseKeyID(t, fixture.InitialKeyID),
		Signature: identity.Signature{
			Algorithm: identity.SigningAlgorithm(fixture.SignatureAlgorithm),
			KeyID:     parseKeyID(t, fixture.SignatureKeyID),
			Bytes:     decodeHex(t, fixture.SignatureHex),
		},
	}
	return Transaction{
		SchemaVersion:  protocol.SchemaVersion(fixture.SchemaVersion),
		NetworkID:      protocol.NetworkID(fixture.NetworkID),
		Type:           TransactionType(fixture.TransactionType),
		IdentityCreate: &proof,
	}
}

func applyIdentityFixture(t *testing.T) (State, Transaction, identityCreateGolden) {
	t.Helper()
	fixture := loadGolden[identityCreateGolden](t, "identity_create_golden.json")
	tx := transactionFromIdentityFixture(t, fixture)
	canonical, err := tx.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, decodeHex(t, fixture.CanonicalTransactionCBORHex)) {
		t.Fatal("IdentityCreate canonical CBOR differs from stored fixture")
	}
	txID, err := tx.ID()
	if err != nil {
		t.Fatal(err)
	}
	if txID.String() != fixture.TxID {
		t.Fatalf("IdentityCreate TxID=%q, want %q", txID.String(), fixture.TxID)
	}
	state, receipt, err := Apply(Genesis(protocol.NetworkID(fixture.NetworkID)), tx)
	if err != nil {
		t.Fatal(err)
	}
	if string(receipt.Code) != fixture.ExpectedResultCode || receipt.StateHash.String() != fixture.ExpectedStateHash || receipt.TxID.String() != fixture.TxID {
		t.Fatalf("IdentityCreate receipt differs from fixture: %+v", receipt)
	}
	return state, tx, fixture
}

func TestIdentityCreateGoldenVector(t *testing.T) {
	state, _, fixture := applyIdentityFixture(t)
	stored := state.Identities[fixture.IdentityID]
	if stored.ID.String() != fixture.IdentityID || !stored.Active[fixture.InitialKeyID] || state.Memberships[fixture.IdentityID] != membership.Pending {
		t.Fatal("IdentityCreate logical result differs from fixture")
	}
}

func transactionFromRotationFixture(t *testing.T, fixture keyRotationGolden) Transaction {
	t.Helper()
	if fixture.FixtureNotice != publicFixtureNotice {
		t.Fatal("rotation fixture safety notice changed")
	}
	proof := identity.RotationProof{
		Request: identity.RotationRequest{
			SchemaVersion: protocol.SchemaVersion(fixture.RotationSchemaVersion),
			NetworkID:     protocol.NetworkID(fixture.NetworkID),
			IdentityID:    parseIdentityID(t, fixture.IdentityID),
			Sequence:      fixture.RotationSequence,
			OldKeyID:      parseKeyID(t, fixture.OldKeyID),
			NewPublicKey: identity.PublicKey{
				Algorithm: identity.SigningAlgorithm(fixture.NewPublicKeyAlgorithm),
				Key:       decodeHex(t, fixture.NewPublicKeyHex),
			},
			CreatedAt: protocol.ProtocolTimestamp(fixture.CreatedAt),
		},
		OldAuthorization: identity.Signature{
			Algorithm: identity.SigningAlgorithm(fixture.OldAuthorizationAlgorithm),
			KeyID:     parseKeyID(t, fixture.OldAuthorizationKeyID),
			Bytes:     decodeHex(t, fixture.OldAuthorizationSignatureHex),
		},
		NewKeyProof: identity.Signature{
			Algorithm: identity.SigningAlgorithm(fixture.NewKeyProofAlgorithm),
			KeyID:     parseKeyID(t, fixture.NewKeyProofKeyID),
			Bytes:     decodeHex(t, fixture.NewKeyProofSignatureHex),
		},
	}
	return Transaction{
		SchemaVersion: protocol.SchemaVersion(fixture.SchemaVersion),
		NetworkID:     protocol.NetworkID(fixture.NetworkID),
		Type:          TransactionType(fixture.TransactionType),
		KeyRotation:   &proof,
	}
}

func applyRotationFixture(t *testing.T) (State, keyRotationGolden) {
	t.Helper()
	state, _, _ := applyIdentityFixture(t)
	fixture := loadGolden[keyRotationGolden](t, "key_rotation_golden.json")
	tx := transactionFromRotationFixture(t, fixture)
	derivedNewKeyID, err := identity.DeriveKeyID(tx.KeyRotation.Request.NewPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if derivedNewKeyID.String() != fixture.NewKeyID {
		t.Fatal("logical new public key does not derive the fixture new KeyID")
	}
	canonical, err := tx.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, decodeHex(t, fixture.CanonicalTransactionCBORHex)) {
		t.Fatal("KeyRotation canonical CBOR differs from stored fixture")
	}
	txID, err := tx.ID()
	if err != nil {
		t.Fatal(err)
	}
	if txID.String() != fixture.TxID {
		t.Fatalf("KeyRotation TxID=%q, want %q", txID.String(), fixture.TxID)
	}
	state, receipt, err := Apply(state, tx)
	if err != nil {
		t.Fatal(err)
	}
	if string(receipt.Code) != fixture.ExpectedResultCode || receipt.StateHash.String() != fixture.ExpectedStateHash || receipt.TxID.String() != fixture.TxID {
		t.Fatalf("KeyRotation receipt differs from fixture: %+v", receipt)
	}
	return state, fixture
}

func TestKeyRotationGoldenVector(t *testing.T) {
	state, fixture := applyRotationFixture(t)
	stored := state.Identities[fixture.IdentityID]
	if stored.Sequence != fixture.RotationSequence || stored.Active[fixture.OldKeyID] || !stored.Active[fixture.NewKeyID] || state.Memberships[fixture.IdentityID] != membership.Pending {
		t.Fatal("KeyRotation logical result differs from fixture")
	}
}

func TestCanonicalStateGoldenVector(t *testing.T) {
	state, rotationFixture := applyRotationFixture(t)
	fixture := loadGolden[stateGolden](t, "state_golden.json")
	if fixture.FixtureNotice != publicFixtureNotice {
		t.Fatal("state fixture safety notice changed")
	}
	if fixture.IdentityID != rotationFixture.IdentityID || fixture.NetworkID != string(state.NetworkID) || fixture.ProtocolVersion != string(state.ProtocolVersion) || fixture.SchemaVersion != uint16(state.SchemaVersion) {
		t.Fatal("logical state description differs from applied state")
	}
	if string(state.Memberships[fixture.IdentityID]) != fixture.MembershipStatus || state.Identities[fixture.IdentityID].Sequence != fixture.RotationSequence {
		t.Fatal("logical membership or sequence differs from state fixture")
	}
	canonical, err := state.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, decodeHex(t, fixture.CanonicalSnapshotCBORHex)) {
		t.Fatal("canonical Snapshot CBOR differs from stored fixture")
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != fixture.SHA256DigestHex {
		t.Fatalf("snapshot SHA-256=%x, want %s", digest, fixture.SHA256DigestHex)
	}
	stateHash, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if stateHash.String() != fixture.StateHash {
		t.Fatalf("StateHash=%q, want %q", stateHash.String(), fixture.StateHash)
	}
}
