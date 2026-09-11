package identity

import (
	"github.com/kokora3/zion/internal/protocol"
	"testing"
)

func validRotation(t *testing.T) (RotationProof, PublicKey) {
	t.Helper()
	g := genesis()
	id, _ := DeriveIdentityID(g)
	old := PublicFromPrivate(oldKey())
	k, _ := DeriveKeyID(old)
	r := RotationRequest{RotationSchema, protocol.Alpha1NetworkID, id, 1, k, PublicFromPrivate(newKey()), 1700000001123}
	p, e := CreateRotation(r, oldKey(), newKey())
	if e != nil {
		t.Fatal(e)
	}
	return p, old
}
func TestRotationSecurityMatrix(t *testing.T) {
	p, old := validRotation(t)
	if VerifyRotation(p, old) != nil {
		t.Fatal("valid")
	}
	cases := []struct {
		name   string
		mutate func(*RotationProof)
		key    PublicKey
	}{
		{"missing-old", func(x *RotationProof) { x.OldAuthorization = Signature{} }, old}, {"missing-new", func(x *RotationProof) { x.NewKeyProof = Signature{} }, old}, {"identity", func(x *RotationProof) { x.Request.IdentityID.Digest[0] ^= 1 }, old}, {"sequence", func(x *RotationProof) { x.Request.Sequence++ }, old}, {"network", func(x *RotationProof) { x.Request.NetworkID = "other" }, old}, {"new-key", func(x *RotationProof) { x.Request.NewPublicKey.Key[0] ^= 1 }, old}, {"timestamp", func(x *RotationProof) { x.Request.CreatedAt++ }, old}, {"old-signature", func(x *RotationProof) { x.OldAuthorization.Bytes[0] ^= 1 }, old}, {"new-signature", func(x *RotationProof) { x.NewKeyProof.Bytes[0] ^= 1 }, old}, {"wrong-old", func(x *RotationProof) {}, PublicFromPrivate(newKey())},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := p
			q.Request.IdentityID.Digest = append([]byte(nil), p.Request.IdentityID.Digest...)
			q.Request.NewPublicKey.Key = append([]byte(nil), p.Request.NewPublicKey.Key...)
			q.OldAuthorization.Bytes = append([]byte(nil), p.OldAuthorization.Bytes...)
			q.NewKeyProof.Bytes = append([]byte(nil), p.NewKeyProof.Bytes...)
			c.mutate(&q)
			if VerifyRotation(q, c.key) == nil {
				t.Fatal("accepted")
			}
		})
	}
}
func TestDomainMatrix(t *testing.T) {
	p, old := validRotation(t)
	g := genesis()
	cp, _ := CreateIdentity(g, oldKey())
	if VerifyIdentityGenesis(cp) != nil {
		t.Fatal("create")
	}
	if verify(p.Request.NetworkID, "zion.identity.key-rotation.new-key-proof", p.Request, old, p.OldAuthorization) == nil {
		t.Fatal("old as new")
	}
	if verify("", "zion.identity.create", g, g.InitialPublicKey, p.NewKeyProof) == nil {
		t.Fatal("new as create")
	}
	if verify("", "zion.identity.create", g, g.InitialPublicKey, p.OldAuthorization) == nil {
		t.Fatal("old as create")
	}
}

func TestRotationRejectsWrongNewSigner(t *testing.T) {
	p, old := validRotation(t)
	// The declared new public key remains unchanged, but the proof is signed by
	// the old private key. Verification must fail cryptographically.
	proof, err := sign(p.Request.NetworkID, "zion.identity.key-rotation.new-key-proof", p.Request, oldKey())
	if err != nil {
		t.Fatal(err)
	}
	p.NewKeyProof = proof
	if VerifyRotation(p, old) == nil {
		t.Fatal("accepted new-key proof signed by a different private key")
	}
}
func FuzzPublicKeyValidate(f *testing.F) {
	f.Add("ed25519", make([]byte, 32))
	f.Add("ed25519", []byte{})
	f.Add("other", make([]byte, 32))
	f.Fuzz(func(t *testing.T, a string, b []byte) { _ = PublicKey{SigningAlgorithm(a), b}.Validate() })
}
func FuzzSignatureValidate(f *testing.F) {
	f.Add("ed25519", make([]byte, 64))
	f.Add("ed25519", []byte{})
	f.Add("other", make([]byte, 64))
	f.Fuzz(func(t *testing.T, a string, b []byte) {
		_ = Signature{Algorithm: SigningAlgorithm(a), Bytes: b}.Validate()
	})
}
