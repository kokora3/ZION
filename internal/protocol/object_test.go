package protocol

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func goldenCore(metadata map[string]string, created ProtocolTimestamp) UnsignedObjectCore {
	return UnsignedObjectCore{ObjectType: "note", SchemaVersion: UnsignedObjectSchemaV1, CreatedAt: created, ContentHash: NewContentHash([]byte("payload")), SizeBytes: 7, Visibility: VisibilityPublic, Metadata: metadata}
}

func TestCanonicalDeterminism(t *testing.T) {
	core := goldenCore(map[string]string{"topic": "AI security", "title": "Cafe\u0301"}, 1710000000123)
	want, err := core.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	id, err := core.ObjectID()
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", want); got != "a701646e6f74650201031b0000018e23f14c7b04a20166736861323536025820239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5050706665055424c494307a2657469746c6565436166c3a965746f7069636b4149207365637572697479" {
		t.Fatalf("golden CBOR changed: %s", got)
	}
	if got := id.String(); got != "zion:obj:sha256:74c2b86bf20b6c02862c8c7f6bbe532ce6d801c999c4a242253784af35f495c3" {
		t.Fatalf("golden ID changed: %s", got)
	}
	for i := 0; i < 128; i++ {
		got, err := core.CanonicalBytes()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("encoding %d changed: %v", i, err)
		}
	}
}
func TestMapOrderAndUnicodeAreCanonical(t *testing.T) {
	a := goldenCore(map[string]string{"title": "Café", "topic": "AI"}, 1)
	b := goldenCore(map[string]string{"topic": "AI", "title": "Cafe\u0301"}, 1)
	ab, _ := a.CanonicalBytes()
	bb, _ := b.CanonicalBytes()
	if !bytes.Equal(ab, bb) {
		t.Fatal("equivalent metadata encoded differently")
	}
}
func TestTimestampTimezoneInvariant(t *testing.T) {
	a := TimestampFromTime(time.Date(2024, 3, 9, 16, 0, 0, 123000000, time.FixedZone("west", -8*3600)))
	b := TimestampFromTime(time.Date(2024, 3, 10, 0, 0, 0, 123000000, time.UTC))
	if a != b || !a.Time().Equal(b.Time()) || a.Time().Location() != time.UTC {
		t.Fatal("timestamp is timezone dependent")
	}
}
func TestObjectIDChangesWithLogicalData(t *testing.T) {
	a := goldenCore(map[string]string{"title": "one"}, 1)
	b := goldenCore(map[string]string{"title": "two"}, 1)
	aid, _ := a.ObjectID()
	bid, _ := b.ObjectID()
	if bytes.Equal(aid.Digest, bid.Digest) {
		t.Fatal("object IDs did not change")
	}
}
func TestObjectIDParser(t *testing.T) {
	id, err := goldenCore(map[string]string{"x": "y"}, 1).ObjectID()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseObjectID(id.String())
	if err != nil || got.String() != id.String() {
		t.Fatalf("round trip: %v", err)
	}
	for _, s := range []string{"bad:obj:sha256:00", "zion:obj:sha512:00", "zion:obj:sha256:00", "zion:obj:sha256:GG", "zion:obj:sha256:ABC", "zion:obj:sha256:aa:extra"} {
		if _, err := ParseObjectID(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}
func TestDecodeRejectsUnsupportedSchemaAndUnknownField(t *testing.T) {
	unsupported := goldenCore(nil, 1)
	unsupported.SchemaVersion = 2
	raw, err := CanonicalEncode(unsupported)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeUnsignedObjectCore(raw); err == nil {
		t.Fatal("accepted schema 2")
	}
	if err := CanonicalDecode([]byte{0xa1, 0x18, 0x63, 0x01}, &UnsignedObjectCore{}); err == nil {
		t.Fatal("accepted unknown field")
	}
}
func FuzzParseObjectID(f *testing.F) {
	f.Add("zion:obj:sha256:0000000000000000000000000000000000000000000000000000000000000000")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) { _, _ = ParseObjectID(s) })
}
func FuzzDecodeUnsignedObjectCore(f *testing.F) {
	f.Add([]byte{0xa0})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = DecodeUnsignedObjectCore(b) })
}
