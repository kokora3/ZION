package research

import (
	"bytes"
	"testing"

	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/registry"
)

func testBody() EntryBody {
	return EntryBody{SchemaVersion: Schema, Title: "Deterministic Research", Summary: "A compact canonical record.",
		Contributors: []Contributor{{DisplayName: "Alice"}}, ExternalIdentifiers: []ExternalIdentifier{{Scheme: DOI, Value: "10.1000/zion"}},
		ExternalURI: "https://example.org/research", ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: []registry.CanonicalReference{}}
}

func TestResearchCanonicalIDAndValidation(t *testing.T) {
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
		t.Fatal("ResearchID round trip failed")
	}
	changed := body
	changed.Title += "!"
	changedID, _ := changed.ID()
	if changedID.String() == id.String() {
		t.Fatal("body change did not change ResearchID")
	}
	for _, invalid := range []string{"", "zion:resource:sha256:" + string(bytes.Repeat([]byte{'0'}, 64)), "zion:research:sha256:AA"} {
		if _, err := ParseID(invalid); err == nil {
			t.Fatalf("accepted invalid ID %q", invalid)
		}
	}
	nonCanonical := body
	nonCanonical.Title = "e\u0301"
	if nonCanonical.Validate() == nil {
		t.Fatal("non-NFC text accepted")
	}
	badURI := body
	badURI.ExternalURI = "http://example.org"
	if badURI.Validate() == nil {
		t.Fatal("non-HTTPS URI accepted")
	}
}

func FuzzParseResearchID(f *testing.F) {
	id, _ := testBody().ID()
	f.Add(id.String())
	f.Add("zion:research:sha256:../")
	f.Fuzz(func(t *testing.T, value string) {
		parsed, err := ParseID(value)
		if err == nil && parsed.String() != value {
			t.Fatal("accepted non-canonical ID")
		}
	})
}

func FuzzDecodeResearchEntry(f *testing.F) {
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
