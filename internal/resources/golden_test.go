package resources

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResourceEntryGolden(t *testing.T) {
	var fixture struct {
		Logical   EntryBody `json:"logical"`
		Canonical string    `json:"canonical_cbor_hex"`
		ID        string    `json:"resource_id"`
	}
	data, err := os.ReadFile(filepath.Join("testdata", "resource_entry_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("decode resource golden")
	}
	canonical, err := fixture.Logical.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(canonical) != fixture.Canonical {
		t.Fatal("resource canonical bytes changed")
	}
	id, _ := fixture.Logical.ID()
	if id.String() != fixture.ID {
		t.Fatal("ResourceID changed")
	}
	if _, err := Decode(canonical); err != nil {
		t.Fatal(err)
	}
}

func TestResourceIDGolden(t *testing.T) {
	var fixture struct {
		ID     string `json:"canonical_id"`
		Digest string `json:"digest_hex"`
	}
	data, err := os.ReadFile(filepath.Join("testdata", "resource_id_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("decode ResourceID golden")
	}
	id, err := ParseID(fixture.ID)
	if err != nil || hex.EncodeToString(id.Digest) != fixture.Digest {
		t.Fatal("ResourceID golden changed")
	}
}
