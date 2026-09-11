package identity

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/kokora3/zion/internal/protocol"
)

func fixture(t *testing.T, n string) map[string]any {
	t.Helper()
	b, e := os.ReadFile("testdata/" + n)
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func hx(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func oldKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
}
func newKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed([]byte{32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63})
}
func genesis() IdentityGenesisBody {
	return IdentityGenesisBody{GenesisSchema, PublicFromPrivate(oldKey()), 1700000000123}
}
func TestGoldenPublicKey(t *testing.T) {
	f := fixture(t, "public_key_golden.json")
	p := PublicFromPrivate(oldKey())
	b, _ := protocol.CanonicalEncode(p)
	if hex.EncodeToString(b) != f["canonical_cbor_hex"] {
		t.Fatal("CBOR mismatch")
	}
	h := protocol.HashBytes(b)
	if hex.EncodeToString(h.Digest) != f["digest_hex"] {
		t.Fatal("hash mismatch")
	}
	id, _ := DeriveKeyID(p)
	if id.String() != f["expected_key_id"] {
		t.Fatal("key ID mismatch")
	}
	if _, e := ParseKeyID(id.String()); e != nil {
		t.Fatal(e)
	}
}
func TestGoldenGenesisAndCreation(t *testing.T) {
	g := genesis()
	f := fixture(t, "identity_genesis_golden.json")
	b, _ := protocol.CanonicalEncode(g)
	if hex.EncodeToString(b) != f["canonical_cbor_hex"] {
		t.Fatal("genesis CBOR")
	}
	id, _ := DeriveIdentityID(g)
	if id.String() != f["expected_identity_id"] {
		t.Fatal("identity ID")
	}
	changed := g
	changed.CreatedAt++
	other, _ := DeriveIdentityID(changed)
	if other.String() == id.String() {
		t.Fatal("immutable change did not change ID")
	}
	pf := fixture(t, "identity_creation_proof_golden.json")
	kid, _ := DeriveKeyID(g.InitialPublicKey)
	sig := Signature{Ed25519, kid, hx(t, pf["signature_hex"].(string))}
	proof := IdentityGenesisProof{g, id, kid, sig}
	if VerifyIdentityGenesis(proof) != nil {
		t.Fatal("stored proof")
	}
	proof.Body.CreatedAt++
	if VerifyIdentityGenesis(proof) == nil {
		t.Fatal("tamper accepted")
	}
	if verify("", "wrong", g, g.InitialPublicKey, sig) == nil {
		t.Fatal("domain replay")
	}
}
func TestGoldenRotationSecurityMatrix(t *testing.T) {
	f := fixture(t, "key_rotation_golden.json")
	g := genesis()
	id, _ := DeriveIdentityID(g)
	old := PublicFromPrivate(oldKey())
	new := PublicFromPrivate(newKey())
	kid, _ := DeriveKeyID(old)
	r := RotationRequest{RotationSchema, protocol.Alpha1NetworkID, id, 1, kid, new, 1700000001123}
	b, _ := protocol.CanonicalEncode(r)
	if hex.EncodeToString(b) != f["canonical_rotation_request_hex"] {
		t.Fatal("rotation CBOR")
	}
	oldSig := f["old_authorization"].(map[string]any)["signature_hex"].(string)
	newSig := f["new_key_proof"].(map[string]any)["signature_hex"].(string)
	p := RotationProof{r, Signature{Ed25519, kid, hx(t, oldSig)}, Signature{Ed25519, mustKey(t, new), hx(t, newSig)}}
	if VerifyRotation(p, old) != nil {
		t.Fatal("stored rotation")
	}
	bad := p
	bad.Request.Sequence++
	if VerifyRotation(bad, old) == nil {
		t.Fatal("sequence tamper")
	}
	bad = p
	bad.NewKeyProof.Bytes[0] ^= 1
	if VerifyRotation(bad, old) == nil {
		t.Fatal("proof tamper")
	}
	if verify("other", "zion.identity.key-rotation.old-key", r, old, p.OldAuthorization) == nil {
		t.Fatal("network replay")
	}
	if verify(r.NetworkID, "zion.identity.key-rotation.new-key-proof", r, old, p.OldAuthorization) == nil {
		t.Fatal("purpose replay")
	}
}
func mustKey(t *testing.T, p PublicKey) KeyID {
	t.Helper()
	k, e := DeriveKeyID(p)
	if e != nil {
		t.Fatal(e)
	}
	return k
}
