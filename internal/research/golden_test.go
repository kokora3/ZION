package research

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResearchEntryGolden(t *testing.T) {
	var fixture struct {
		Logical   EntryBody `json:"logical"`
		Canonical string    `json:"canonical_cbor_hex"`
		ID        string    `json:"research_id"`
	}
	data, err := os.ReadFile(filepath.Join("testdata", "research_entry_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("decode research golden")
	}
	canonical, err := fixture.Logical.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(canonical) != fixture.Canonical {
		t.Fatal("research canonical bytes changed")
	}
	id, _ := fixture.Logical.ID()
	if id.String() != fixture.ID {
		t.Fatal("ResearchID changed")
	}
	if _, err := Decode(canonical); err != nil {
		t.Fatal(err)
	}
}

func TestResearchIDGolden(t *testing.T) {
	var fixture struct {
		ID     string `json:"canonical_id"`
		Digest string `json:"digest_hex"`
	}
	data, err := os.ReadFile(filepath.Join("testdata", "research_id_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("decode ResearchID golden")
	}
	id, err := ParseID(fixture.ID)
	if err != nil || hex.EncodeToString(id.Digest) != fixture.Digest {
		t.Fatal("ResearchID golden changed")
	}
}
