package identity

import (
	"encoding/json"
	"github.com/kokora3/zion/internal/protocol"
	"os"
	"testing"
)

func TestGoldenFixturesAreValidJSON(t *testing.T) {
	for _, name := range []string{"public_key_golden.json", "identity_genesis_golden.json", "identity_creation_proof_golden.json", "key_rotation_golden.json"} {
		data, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var fixture map[string]any
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, field := range []string{"fixture", "warning"} {
			if fixture[field] == nil {
				t.Fatalf("%s missing %s", name, field)
			}
		}
	}
}

func TestIdentityAndRotation(t *testing.T) {
	pa, a, _ := GenerateEd25519Key()
	pb, b, _ := GenerateEd25519Key()
	body := IdentityGenesisBody{GenesisSchema, PublicKey{Ed25519, pa}, 1}
	proof, e := CreateIdentity(body, a)
	if e != nil || VerifyIdentityGenesis(proof) != nil {
		t.Fatal("genesis proof failed")
	}
	id := proof.IdentityID
	ka, _ := DeriveKeyID(PublicKey{Ed25519, pa})
	r := RotationRequest{RotationSchema, protocol.Alpha1NetworkID, id, 1, ka, PublicKey{Ed25519, pb}, 2}
	rp, e := CreateRotation(r, a, b)
	if e != nil || VerifyRotation(rp, PublicKey{Ed25519, pa}) != nil {
		t.Fatal("rotation failed")
	}
	rp.Request.Sequence = 2
	if VerifyRotation(rp, PublicKey{Ed25519, pa}) == nil {
		t.Fatal("tampered rotation accepted")
	}
	if id.String() == "" {
		t.Fatal("empty ID")
	}
}
func TestDomainAndNetworkSeparation(t *testing.T) {
	p, k, _ := GenerateEd25519Key()
	body := IdentityGenesisBody{GenesisSchema, PublicKey{Ed25519, p}, 1}
	s, _ := sign(protocol.Alpha1NetworkID, "a", body, k)
	if verify(protocol.Alpha1NetworkID, "b", body, PublicKey{Ed25519, p}, s) == nil || verify("other", "a", body, PublicKey{Ed25519, p}, s) == nil {
		t.Fatal("replay accepted")
	}
}
func FuzzParseIdentityID(f *testing.F) {
	f.Add("zion:id:sha256:0000000000000000000000000000000000000000000000000000000000000000")
	f.Fuzz(func(t *testing.T, s string) { _, _ = ParseIdentityID(s) })
}
func FuzzParseKeyID(f *testing.F) {
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) { _, _ = ParseKeyID(s) })
}
