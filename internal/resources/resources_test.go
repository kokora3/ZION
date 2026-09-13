package resources

import (
	"bytes"
	"testing"

	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
)

func testBody() EntryBody {
	return EntryBody{SchemaVersion: Schema, Kind: Dataset, Name: "ZION Dataset", Summary: "A compact canonical resource.",
		ExternalURI: "https://example.org/data", License: "CC0-1.0", Maintainers: []Maintainer{{DisplayName: "Bob"}},
		ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{}}
}

func TestResourceCanonicalIDAndValidation(t *testing.T) {
	body := testBody()
	canonical, err := body.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(canonical)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := decoded.CanonicalBytes()
	if !bytes.Equal(canonical, again) {
		t.Fatal("canonical round trip changed bytes")
	}
	id, _ := body.ID()
	parsed, err := ParseID(id.String())
	if err != nil || parsed.String() != id.String() {
		t.Fatal("ResourceID round trip failed")
	}
	changed := body
	changed.Kind = Tool
	changedID, _ := changed.ID()
	if changedID.String() == id.String() {
		t.Fatal("body change did not change ResourceID")
	}
	for _, invalid := range []string{"", "zion:research:sha256:" + string(bytes.Repeat([]byte{'0'}, 64)), "zion:resource:sha256:AA"} {
		if _, err := ParseID(invalid); err == nil {
			t.Fatalf("accepted invalid ID %q", invalid)
		}
	}
	bad := body
	bad.CanonicalRefs = []registry.CanonicalReference{{Relation: "x", TargetKind: "UNKNOWN", TargetID: "bad"}}
	if bad.Validate() == nil {
		t.Fatal("invalid reference accepted")
	}
}

func FuzzParseResourceID(f *testing.F) {
	id, _ := testBody().ID()
	f.Add(id.String())
	f.Add("zion:resource:sha256:../")
	f.Fuzz(func(t *testing.T, value string) {
		parsed, err := ParseID(value)
		if err == nil && parsed.String() != value {
			t.Fatal("accepted non-canonical ID")
		}
	})
}

func FuzzDecodeResourceEntry(f *testing.F) {
	valid, _ := testBody().CanonicalBytes()
	f.Add(valid)
	f.Add([]byte{0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		body, err := Decode(data)
		if err == nil {
			canonical, encodeErr := body.CanonicalBytes()
			if encodeErr != nil || !bytes.Equal(canonical, data) {
				t.Fatal("accepted ambiguous encoding")
			}
		}
	})
}
