// Package identity provides cryptographic identity primitives only. Chain-state
// authorization, persistence, and P2P peer identities are intentionally absent.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/kokora3/zion/internal/protocol"
)

type SigningAlgorithm string

const Ed25519 SigningAlgorithm = "ed25519"
const (
	GenesisSchema  protocol.SchemaVersion = 1
	RotationSchema protocol.SchemaVersion = 1
	MaxActiveKeys                         = 16
)

type PublicKey struct {
	Algorithm SigningAlgorithm `cbor:"1,keyasint"`
	Key       []byte           `cbor:"2,keyasint"`
}

func (p PublicKey) Validate() error {
	if p.Algorithm != Ed25519 {
		return fmt.Errorf("unknown signing algorithm %q", p.Algorithm)
	}
	if len(p.Key) != ed25519.PublicKeySize {
		return fmt.Errorf("ed25519 public key length %d", len(p.Key))
	}
	return nil
}
func PublicFromPrivate(k ed25519.PrivateKey) PublicKey {
	return PublicKey{Ed25519, k.Public().(ed25519.PublicKey)}
}
func GenerateEd25519Key() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

type KeyID struct{ protocol.HashDigest }

func (id KeyID) String() string {
	if id.Validate() != nil {
		return ""
	}
	return "zion:key:sha256:" + hex.EncodeToString(id.Digest)
}
func (id KeyID) Validate() error { return id.HashDigest.Validate() }
func DeriveKeyID(p PublicKey) (KeyID, error) {
	if err := p.Validate(); err != nil {
		return KeyID{}, err
	}
	h, e := protocol.HashCanonical(p)
	return KeyID{h}, e
}
func ParseKeyID(s string) (KeyID, error) {
	d, e := parseID(s, "key")
	if e != nil {
		return KeyID{}, e
	}
	return KeyID{d}, nil
}

type IdentityID struct{ protocol.HashDigest }

func (id IdentityID) String() string {
	if id.Validate() != nil {
		return ""
	}
	return "zion:id:sha256:" + hex.EncodeToString(id.Digest)
}
func (id IdentityID) Validate() error { return id.HashDigest.Validate() }
func ParseIdentityID(s string) (IdentityID, error) {
	d, e := parseID(s, "id")
	if e != nil {
		return IdentityID{}, e
	}
	return IdentityID{d}, nil
}
func parseID(s, kind string) (protocol.HashDigest, error) {
	p := strings.Split(s, ":")
	if len(p) != 4 || p[0] != "zion" || p[1] != kind || p[2] != "sha256" || strings.ToLower(p[3]) != p[3] {
		return protocol.HashDigest{}, fmt.Errorf("invalid %s ID", kind)
	}
	d, e := hex.DecodeString(p[3])
	if e != nil {
		return protocol.HashDigest{}, e
	}
	h := protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: d}
	return h, h.Validate()
}

type IdentityGenesisBody struct {
	SchemaVersion    protocol.SchemaVersion     `cbor:"1,keyasint"`
	InitialPublicKey PublicKey                  `cbor:"2,keyasint"`
	CreatedAt        protocol.ProtocolTimestamp `cbor:"3,keyasint"`
}

func (b IdentityGenesisBody) Validate() error {
	if b.SchemaVersion != GenesisSchema {
		return fmt.Errorf("unsupported genesis schema")
	}
	return b.InitialPublicKey.Validate()
}
func DeriveIdentityID(b IdentityGenesisBody) (IdentityID, error) {
	if e := b.Validate(); e != nil {
		return IdentityID{}, e
	}
	h, e := protocol.HashCanonical(b)
	return IdentityID{h}, e
}

type Signature struct {
	Algorithm SigningAlgorithm `cbor:"1,keyasint"`
	KeyID     KeyID            `cbor:"2,keyasint"`
	Bytes     []byte           `cbor:"3,keyasint"`
}

func (s Signature) Validate() error {
	if s.Algorithm != Ed25519 {
		return fmt.Errorf("unknown signing algorithm")
	}
	if e := s.KeyID.Validate(); e != nil {
		return e
	}
	if len(s.Bytes) != ed25519.SignatureSize {
		return fmt.Errorf("signature length %d", len(s.Bytes))
	}
	return nil
}

type signingEnvelope struct {
	Domain        string                 `cbor:"1,keyasint"`
	SchemaVersion protocol.SchemaVersion `cbor:"2,keyasint"`
	NetworkID     protocol.NetworkID     `cbor:"3,keyasint"`
	Purpose       string                 `cbor:"4,keyasint"`
	Payload       any                    `cbor:"5,keyasint"`
}

func sign(network protocol.NetworkID, purpose string, payload any, key ed25519.PrivateKey) (Signature, error) {
	p := PublicFromPrivate(key)
	kid, e := DeriveKeyID(p)
	if e != nil {
		return Signature{}, e
	}
	b, e := protocol.CanonicalEncode(signingEnvelope{"zion.signature", 1, network, purpose, payload})
	if e != nil {
		return Signature{}, e
	}
	return Signature{Ed25519, kid, ed25519.Sign(key, b)}, nil
}
func verify(network protocol.NetworkID, purpose string, payload any, p PublicKey, s Signature) error {
	if e := p.Validate(); e != nil {
		return e
	}
	if e := s.Validate(); e != nil {
		return e
	}
	kid, e := DeriveKeyID(p)
	if e != nil || kid.String() != s.KeyID.String() {
		return fmt.Errorf("signer key mismatch")
	}
	b, e := protocol.CanonicalEncode(signingEnvelope{"zion.signature", 1, network, purpose, payload})
	if e != nil {
		return e
	}
	if !ed25519.Verify(ed25519.PublicKey(p.Key), b, s.Bytes) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type IdentityGenesisProof struct {
	Body         IdentityGenesisBody `cbor:"1,keyasint"`
	IdentityID   IdentityID          `cbor:"2,keyasint"`
	InitialKeyID KeyID               `cbor:"3,keyasint"`
	Signature    Signature           `cbor:"4,keyasint"`
}

func CreateIdentity(b IdentityGenesisBody, key ed25519.PrivateKey) (IdentityGenesisProof, error) {
	id, e := DeriveIdentityID(b)
	if e != nil {
		return IdentityGenesisProof{}, e
	}
	kid, e := DeriveKeyID(b.InitialPublicKey)
	if e != nil {
		return IdentityGenesisProof{}, e
	}
	s, e := sign("", "zion.identity.create", b, key)
	return IdentityGenesisProof{b, id, kid, s}, e
}
func VerifyIdentityGenesis(p IdentityGenesisProof) error {
	id, e := DeriveIdentityID(p.Body)
	if e != nil || id.String() != p.IdentityID.String() {
		return fmt.Errorf("identity ID mismatch")
	}
	kid, e := DeriveKeyID(p.Body.InitialPublicKey)
	if e != nil || kid.String() != p.InitialKeyID.String() {
		return fmt.Errorf("key ID mismatch")
	}
	return verify("", "zion.identity.create", p.Body, p.Body.InitialPublicKey, p.Signature)
}

type RotationRequest struct {
	SchemaVersion protocol.SchemaVersion     `cbor:"1,keyasint"`
	NetworkID     protocol.NetworkID         `cbor:"2,keyasint"`
	IdentityID    IdentityID                 `cbor:"3,keyasint"`
	Sequence      uint64                     `cbor:"4,keyasint"`
	OldKeyID      KeyID                      `cbor:"5,keyasint"`
	NewPublicKey  PublicKey                  `cbor:"6,keyasint"`
	CreatedAt     protocol.ProtocolTimestamp `cbor:"7,keyasint"`
}
type RotationProof struct {
	Request          RotationRequest `cbor:"1,keyasint"`
	OldAuthorization Signature       `cbor:"2,keyasint"`
	NewKeyProof      Signature       `cbor:"3,keyasint"`
}

func CreateRotation(r RotationRequest, old, new ed25519.PrivateKey) (RotationProof, error) {
	a, e := sign(r.NetworkID, "zion.identity.key-rotation.old-key", r, old)
	if e != nil {
		return RotationProof{}, e
	}
	n, e := sign(r.NetworkID, "zion.identity.key-rotation.new-key-proof", r, new)
	return RotationProof{r, a, n}, e
}
func VerifyRotation(p RotationProof, old PublicKey) error {
	if p.Request.SchemaVersion != RotationSchema {
		return fmt.Errorf("unsupported rotation schema")
	}
	if e := p.Request.IdentityID.Validate(); e != nil {
		return e
	}
	if e := p.Request.NewPublicKey.Validate(); e != nil {
		return e
	}
	kid, e := DeriveKeyID(old)
	if e != nil || kid.String() != p.Request.OldKeyID.String() {
		return fmt.Errorf("old key mismatch")
	}
	if e = verify(p.Request.NetworkID, "zion.identity.key-rotation.old-key", p.Request, old, p.OldAuthorization); e != nil {
		return e
	}
	return verify(p.Request.NetworkID, "zion.identity.key-rotation.new-key-proof", p.Request, p.Request.NewPublicKey, p.NewKeyProof)
}

type KeyState string

const (
	KeyActive  KeyState = "ACTIVE"
	KeyRetired KeyState = "RETIRED"
	KeyRevoked KeyState = "REVOKED"
)

type RevocationState string

const (
	IdentityNotRevoked RevocationState = "NOT_REVOKED"
	IdentityRevoked    RevocationState = "REVOKED"
)

type IdentityRecord struct {
	IdentityID        IdentityID
	ActiveSigningKeys []KeyID
	RetiredKeys       []KeyID
	RotationSequence  uint64
	RevocationState   RevocationState
}
