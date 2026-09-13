package board

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
)

func boardTestIdentity(t testing.TB, marker byte) (ed25519.PrivateKey, identity.IdentityID, identity.KeyID, chain.State) {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	for index := range privateKey[:ed25519.SeedSize] {
		privateKey[index] = marker
	}
	privateKey = ed25519.NewKeyFromSeed(privateKey[:ed25519.SeedSize])
	publicKey := identity.PublicFromPrivate(privateKey)
	body := identity.IdentityGenesisBody{SchemaVersion: identity.GenesisSchema, InitialPublicKey: publicKey, CreatedAt: 1700000000000}
	id, err := identity.DeriveIdentityID(body)
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := identity.DeriveKeyID(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	state := chain.Genesis(protocol.Alpha1NetworkID)
	state.Identities[id.String()] = chain.StoredIdentity{ID: id, Keys: map[string]identity.PublicKey{keyID.String(): publicKey},
		Active: map[string]bool{keyID.String(): true}, Revoked: false}
	state.Memberships[id.String()] = membership.Active
	return privateKey, id, keyID, state
}

func boardTestEvent(t testing.TB) (SignedEvent, chain.State, ed25519.PrivateKey) {
	t.Helper()
	key, id, keyID, state := boardTestIdentity(t, 0x42)
	content, err := BuildContentObject(Content{SchemaVersion: ContentSchema, Format: FormatMarkdown, Title: "ZION Board",
		Body: "สวัสดี ZION decentralized board"}, 1710000000123)
	if err != nil {
		t.Fatal(err)
	}
	contentID, _ := content.ObjectID()
	body := EventBody{SchemaVersion: EventSchema, NetworkID: protocol.Alpha1NetworkID, Kind: KindPost, AuthorIdentity: id,
		AuthorKeyID: keyID, CreatedAt: 1710000000123, ContentObject: contentID.String(), References: []Reference{{
			Relation: "zion.community/references/v1", TargetKind: "OBJECT", TargetID: contentID.String()}}}
	event, err := SignEvent(body, key)
	if err != nil {
		t.Fatal(err)
	}
	return event, state, key
}

func TestBoardContentEventPostIDAndAuthorization(t *testing.T) {
	event, state, _ := boardTestEvent(t)
	status, memberStatus, err := VerifyAndClassify(event, state)
	if err != nil || status != CurrentlyAuthorized || memberStatus != membership.Active {
		t.Fatalf("authorization = %s %s %v", status, memberStatus, err)
	}
	object, err := BuildEventObject(event)
	if err != nil {
		t.Fatal(err)
	}
	id, err := PostID(object)
	if err != nil || id.String() == event.Body.ContentObject {
		t.Fatalf("post id = %s, %v", id.String(), err)
	}
	raw, _ := object.CanonicalBytes()
	decodedObject, err := protocolObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeEventObject(decodedObject)
	if err != nil || decoded.Body.AuthorIdentity.String() != event.Body.AuthorIdentity.String() {
		t.Fatal("Board event object did not round trip")
	}

	stored := state.Identities[event.Body.AuthorIdentity.String()]
	stored.Active[event.Body.AuthorKeyID.String()] = false
	state.Identities[event.Body.AuthorIdentity.String()] = stored
	status, _, err = VerifyAndClassify(event, state)
	if err != nil || status != HistoricalOrUnconfirmed {
		t.Fatalf("retired historical event = %s, %v", status, err)
	}
}

func protocolObject(raw []byte) (object objects.Object, err error) { return objects.Decode(raw) }

func TestBoardSignatureTamperAndDomainMatrix(t *testing.T) {
	event, state, key := boardTestEvent(t)
	mutations := []func(*SignedEvent){
		func(v *SignedEvent) { v.Body.CreatedAt++ },
		func(v *SignedEvent) { v.Body.NetworkID = "other" },
		func(v *SignedEvent) { v.Body.ContentObject = "zion:obj:sha256:" + strings.Repeat("a", 64) },
		func(v *SignedEvent) { v.Body.Kind = KindReply; v.Body.ParentPost = v.Body.ContentObject },
		func(v *SignedEvent) { v.Body.References[0].Relation = "zion.community/discusses/v1" },
		func(v *SignedEvent) { v.Signature.Bytes[0] ^= 1 },
	}
	for index, mutate := range mutations {
		candidate := event
		candidate.Signature.Bytes = append([]byte(nil), event.Signature.Bytes...)
		candidate.Body.References = append([]Reference(nil), event.Body.References...)
		mutate(&candidate)
		if _, _, err := VerifyAndClassify(candidate, state); err == nil {
			t.Fatalf("tamper mutation %d verified", index)
		}
	}
	wrongDomain := event
	wrongDomain.Signature, _ = identity.SignPurpose(event.Body.NetworkID, "zion.governance.vote", event.Body, key)
	if _, _, err := VerifyAndClassify(wrongDomain, state); err == nil {
		t.Fatal("governance-domain signature verified as Board event")
	}
	wrongNetwork := event
	wrongNetwork.Body.NetworkID = "another-network"
	wrongNetwork, _ = SignEvent(wrongNetwork.Body, key)
	if _, _, err := VerifyAndClassify(wrongNetwork, state); !errors.Is(err, ErrPublicationDenied) {
		t.Fatalf("valid wrong-network event error = %v", err)
	}
}

func TestBoardContentAndReferenceLimits(t *testing.T) {
	if _, err := (Content{SchemaVersion: ContentSchema, Format: FormatText, Body: string(make([]byte, MaxBoardContentBodyBytes+1))}).CanonicalBytes(); err == nil {
		t.Fatal("oversized Board content accepted")
	}
	event, _, _ := boardTestEvent(t)
	event.Body.References = make([]Reference, MaxBoardReferences+1)
	if _, err := event.CanonicalBytes(); err == nil {
		t.Fatal("too many Board references accepted")
	}
}

func TestBoardAuthorizationMembershipMatrix(t *testing.T) {
	event, state, _ := boardTestEvent(t)
	for _, memberStatus := range []membership.Status{membership.Pending, membership.Suspended, membership.Revoked} {
		candidate := state
		candidate.Memberships = make(map[string]membership.Status, len(state.Memberships))
		for id, status := range state.Memberships {
			candidate.Memberships[id] = status
		}
		candidate.Memberships[event.Body.AuthorIdentity.String()] = memberStatus
		status, gotMembership, err := VerifyAndClassify(event, candidate)
		if err != nil || status != HistoricalOrUnconfirmed || gotMembership != memberStatus {
			t.Fatalf("membership %s classified as %s/%s: %v", memberStatus, status, gotMembership, err)
		}
	}
	revokedIdentity := state
	revokedIdentity.Identities = make(map[string]chain.StoredIdentity, len(state.Identities))
	for id, stored := range state.Identities {
		revokedIdentity.Identities[id] = stored
	}
	stored := revokedIdentity.Identities[event.Body.AuthorIdentity.String()]
	stored.Revoked = true
	revokedIdentity.Identities[event.Body.AuthorIdentity.String()] = stored
	if status, _, err := VerifyAndClassify(event, revokedIdentity); err != nil || status != HistoricalOrUnconfirmed {
		t.Fatalf("revoked identity classification = %s, %v", status, err)
	}
	unknown := chain.Genesis(protocol.Alpha1NetworkID)
	if _, _, err := VerifyAndClassify(event, unknown); !errors.Is(err, ErrPublicationDenied) {
		t.Fatalf("unknown identity error = %v", err)
	}
}

func FuzzDecodeBoardContent(f *testing.F) {
	seed, _ := (Content{SchemaVersion: ContentSchema, Format: FormatText, Body: "seed"}).CanonicalBytes()
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeContent(data) })
}

func FuzzDecodeBoardEvent(f *testing.F) {
	event, _, _ := boardTestEvent(f)
	seed, _ := event.CanonicalBytes()
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeEvent(data) })
}
