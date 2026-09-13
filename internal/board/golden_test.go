package board

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/protocol"
)

type contentGolden struct {
	SchemaVersion    protocol.SchemaVersion     `json:"schema_version"`
	Format           ContentFormat              `json:"format"`
	Title            string                     `json:"title"`
	Body             string                     `json:"body"`
	CanonicalCBORHex string                     `json:"canonical_cbor_hex"`
	ObjectCBORHex    string                     `json:"object_cbor_hex"`
	ContentObjectID  string                     `json:"content_object_id"`
	CreatedAt        protocol.ProtocolTimestamp `json:"created_at"`
}

type eventGolden struct {
	SchemaVersion    protocol.SchemaVersion     `json:"schema_version"`
	NetworkID        protocol.NetworkID         `json:"network_id"`
	Kind             EventKind                  `json:"kind"`
	AuthorIdentityID string                     `json:"author_identity_id"`
	AuthorKeyID      string                     `json:"author_key_id"`
	CreatedAt        protocol.ProtocolTimestamp `json:"created_at"`
	ParentPostID     string                     `json:"parent_post_id"`
	ContentObjectID  string                     `json:"content_object_id"`
	References       []Reference                `json:"references"`
	SigningPurpose   string                     `json:"signing_purpose"`
	SignatureHex     string                     `json:"signature_hex"`
	CanonicalCBORHex string                     `json:"canonical_cbor_hex"`
	EventObjectHex   string                     `json:"event_object_cbor_hex"`
	PostID           string                     `json:"post_id"`
}

type wireGolden struct {
	SchemaVersion    protocol.SchemaVersion `json:"schema_version"`
	NetworkID        protocol.NetworkID     `json:"network_id"`
	ProtocolID       string                 `json:"protocol_id"`
	PostID           string                 `json:"post_id"`
	AfterPostID      string                 `json:"after_post_id"`
	Limit            uint16                 `json:"limit"`
	PostIDs          []string               `json:"post_ids"`
	NextPostID       string                 `json:"next_post_id"`
	More             bool                   `json:"more"`
	CanonicalCBORHex string                 `json:"canonical_cbor_hex"`
}

func loadGolden[T any](t testing.TB, name string) T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func deterministicGoldenEvent(t testing.TB) (Content, []byte, SignedEvent, []byte) {
	t.Helper()
	contentFixture := loadGolden[contentGolden](t, "board_content_golden.json")
	content := Content{SchemaVersion: contentFixture.SchemaVersion, Format: contentFixture.Format, Title: contentFixture.Title, Body: contentFixture.Body}
	contentObject, err := BuildContentObject(content, contentFixture.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	contentObjectBytes, _ := contentObject.CanonicalBytes()
	key := ed25519.NewKeyFromSeed([]byte("ZION PHASE 10 PUBLIC TEST SEED!!"))
	eventFixture := loadGolden[eventGolden](t, "board_event_golden.json")
	identityID, err := identity.ParseIdentityID(eventFixture.AuthorIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := identity.ParseKeyID(eventFixture.AuthorKeyID)
	if err != nil {
		t.Fatal(err)
	}
	event, err := SignEvent(EventBody{SchemaVersion: eventFixture.SchemaVersion, NetworkID: eventFixture.NetworkID,
		Kind: eventFixture.Kind, AuthorIdentity: identityID, AuthorKeyID: keyID, CreatedAt: eventFixture.CreatedAt,
		ParentPost: eventFixture.ParentPostID, ContentObject: eventFixture.ContentObjectID, References: eventFixture.References}, key)
	if err != nil {
		t.Fatal(err)
	}
	eventObject, err := BuildEventObject(event)
	if err != nil {
		t.Fatal(err)
	}
	eventObjectBytes, _ := eventObject.CanonicalBytes()
	return content, contentObjectBytes, event, eventObjectBytes
}

func TestBoardContentGolden(t *testing.T) {
	fixture := loadGolden[contentGolden](t, "board_content_golden.json")
	content, objectBytes, _, _ := deterministicGoldenEvent(t)
	contentBytes, _ := content.CanonicalBytes()
	if hex.EncodeToString(contentBytes) != fixture.CanonicalCBORHex || hex.EncodeToString(objectBytes) != fixture.ObjectCBORHex {
		t.Fatal("Board content canonical bytes changed")
	}
	object, err := BuildContentObject(content, fixture.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := object.ObjectID()
	if id.String() != fixture.ContentObjectID {
		t.Fatalf("content ObjectID = %s", id.String())
	}
	if decoded, err := DecodeContent(contentBytes); err != nil || decoded != content {
		t.Fatalf("golden content decode = %+v, %v", decoded, err)
	}
}

func TestBoardEventGolden(t *testing.T) {
	fixture := loadGolden[eventGolden](t, "board_event_golden.json")
	_, _, event, objectBytes := deterministicGoldenEvent(t)
	eventBytes, _ := event.CanonicalBytes()
	if fixture.SigningPurpose != SigningPurpose || hex.EncodeToString(event.Signature.Bytes) != fixture.SignatureHex ||
		hex.EncodeToString(eventBytes) != fixture.CanonicalCBORHex || hex.EncodeToString(objectBytes) != fixture.EventObjectHex {
		t.Fatal("Board event signature or canonical bytes changed")
	}
	object, _ := protocolObject(objectBytes)
	postID, err := PostID(object)
	if err != nil || postID.String() != fixture.PostID {
		t.Fatalf("PostID = %s, %v", postID.String(), err)
	}
	decoded, err := DecodeEvent(eventBytes)
	if err != nil || decoded.Body.ContentObject != fixture.ContentObjectID {
		t.Fatalf("golden event decode failed: %v", err)
	}
}

func TestBoardWireGoldens(t *testing.T) {
	_, _, _, eventObjectBytes := deterministicGoldenEvent(t)
	announceFixture := loadGolden[wireGolden](t, "board_announce_golden.json")
	announceBytes, err := EncodeAnnounce(Announce{SchemaVersion: announceFixture.SchemaVersion, NetworkID: announceFixture.NetworkID, EventObject: eventObjectBytes})
	if err != nil || announceFixture.ProtocolID != AnnounceProtocolID || hex.EncodeToString(announceBytes) != announceFixture.CanonicalCBORHex {
		t.Fatalf("announcement golden changed: %v", err)
	}
	decodedAnnounce, err := DecodeAnnounce(announceBytes, protocol.Alpha1NetworkID)
	if err != nil || len(decodedAnnounce.EventObject) == 0 {
		t.Fatalf("announcement golden decode: %v", err)
	}

	requestFixture := loadGolden[wireGolden](t, "board_sync_request_golden.json")
	requestBytes, err := EncodeSyncRequest(SyncRequest{SchemaVersion: requestFixture.SchemaVersion, NetworkID: requestFixture.NetworkID,
		AfterPostID: requestFixture.AfterPostID, Limit: requestFixture.Limit})
	if err != nil || requestFixture.ProtocolID != SyncProtocolID || hex.EncodeToString(requestBytes) != requestFixture.CanonicalCBORHex {
		t.Fatalf("sync request golden changed: %v", err)
	}
	if _, err := DecodeSyncRequest(requestBytes, protocol.Alpha1NetworkID); err != nil {
		t.Fatal(err)
	}

	responseFixture := loadGolden[wireGolden](t, "board_sync_response_golden.json")
	responseBytes, err := EncodeSyncResponse(SyncResponse{SchemaVersion: responseFixture.SchemaVersion, NetworkID: responseFixture.NetworkID,
		Events: [][]byte{eventObjectBytes}, NextPostID: responseFixture.NextPostID, More: responseFixture.More})
	if err != nil || responseFixture.ProtocolID != SyncProtocolID || hex.EncodeToString(responseBytes) != responseFixture.CanonicalCBORHex {
		t.Fatalf("sync response golden changed: %v", err)
	}
	decodedResponse, err := DecodeSyncResponse(responseBytes, protocol.Alpha1NetworkID)
	if err != nil || len(decodedResponse.Events) != len(responseFixture.PostIDs) {
		t.Fatalf("sync response golden decode: %v", err)
	}
}
