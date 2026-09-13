package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

func TestRegistryIndexRebuildSearchRestartAndCorruptRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registries.json")
	index, err := OpenRegistries(path)
	if err != nil {
		t.Fatal(err)
	}
	researchBody := research.EntryBody{SchemaVersion: research.Schema, Title: "Quantum ZION", Summary: "Deterministic registry",
		Contributors: []research.Contributor{{DisplayName: "Alice"}}, ExternalIdentifiers: []research.ExternalIdentifier{},
		ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: nil}
	resourceBody := resources.EntryBody{SchemaVersion: resources.Schema, Kind: resources.Dataset, Name: "Alpha Dataset",
		Summary: "Quantum samples", Maintainers: []resources.Maintainer{{DisplayName: "Bob"}}, ObjectRefs: []protocol.ObjectID{}, CanonicalRefs: nil}
	researchID, _ := researchBody.ID()
	resourceID, _ := resourceBody.ID()
	proposalID := governance.ProposalID{HashDigest: protocol.HashBytes([]byte("proposal"))}
	state := chain.State{Research: map[string]chain.StoredResearch{researchID.String(): {ID: researchID, Body: researchBody, ProposalID: proposalID, AdmittedHeight: 10}},
		Resources: map[string]chain.StoredResource{resourceID.String(): {ID: resourceID, Body: resourceBody, ProposalID: proposalID, AdmittedHeight: 11}}}
	if err := index.Rebuild(state); err != nil {
		t.Fatal(err)
	}
	results, err := index.ResearchSearch("quantum", 0, 10)
	if err != nil || len(results) != 1 {
		t.Fatalf("research search: %v %#v", err, results)
	}
	resourcesFound, err := index.ResourceSearch("samples", 0, 10)
	if err != nil || len(resourcesFound) != 1 {
		t.Fatalf("resource search: %v %#v", err, resourcesFound)
	}
	reopened, err := OpenRegistries(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Research(researchID.String()); !ok {
		t.Fatal("persisted research index missing")
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt, err := OpenRegistries(path)
	if err != nil {
		t.Fatal(err)
	}
	if researchCount, resourceCount := corrupt.Counts(); researchCount != 0 || resourceCount != 0 {
		t.Fatal("corrupt derived index was trusted")
	}
	if err := corrupt.Rebuild(state); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryIndexBounds(t *testing.T) {
	index, _ := OpenRegistries(filepath.Join(t.TempDir(), "index.json"))
	if _, err := index.ResearchList(-1, 1); err == nil {
		t.Fatal("negative offset accepted")
	}
	if _, err := index.ResourceList(0, HardQueryLimit+1); err == nil {
		t.Fatal("oversized page accepted")
	}
	if _, err := index.ResearchSearch("", 0, 1); err == nil {
		t.Fatal("empty query accepted")
	}
}
