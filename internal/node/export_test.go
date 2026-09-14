package node

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/index"
	"github.com/kokora3/zion/internal/protocol"
)

func TestStateExportImportRoundTripAndSafety(t *testing.T) {
	directory := t.TempDir()
	genesis := protocol.HashBytes([]byte("phase-13-export-genesis"))
	state := registryRegistryState(t)
	source := filepath.Join(directory, "source", "application.snapshot")
	store, err := NewStateStore(source, state.NetworkID, genesis)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(state, 42); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(directory, "backup", "state-export.json")
	info, err := ExportState(source, exportPath, state.NetworkID, genesis)
	if err != nil {
		t.Fatal(err)
	}
	wantHash, _ := state.Hash()
	if info.AcceptedHeight != 42 || info.StateHash != wantHash.String() || info.StateSchemaVersion != chain.StateSchemaV3 {
		t.Fatalf("unexpected export metadata: %+v", info)
	}
	if _, err := ExportState(source, exportPath, state.NetworkID, genesis); err == nil {
		t.Fatal("export silently overwrote an existing file")
	}
	if temporary, _ := filepath.Glob(filepath.Join(filepath.Dir(exportPath), ".export-*")); len(temporary) != 0 {
		t.Fatalf("failed export left temporary files: %v", temporary)
	}
	inspected, _, _, err := InspectStateExport(exportPath)
	if err != nil || inspected != info {
		t.Fatalf("inspect failed: %+v %v", inspected, err)
	}
	target := filepath.Join(directory, "fresh", "application.snapshot")
	imported, err := ImportState(exportPath, target, state.NetworkID, genesis)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Migrated || imported.StateHash != wantHash.String() {
		t.Fatalf("unexpected import: %+v", imported)
	}
	loaded, err := mustStateStore(t, target, state.NetworkID, genesis).Load()
	if err != nil {
		t.Fatal(err)
	}
	gotHash, _ := loaded.State.Hash()
	if loaded.Height != 42 || gotHash.String() != wantHash.String() {
		t.Fatal("export/import did not preserve height and StateHash")
	}
	runtimeCfg := testRuntimeConfig(t, "imported-export", false, false)
	runtimeCfg.GenesisID = genesis
	runtimeCfg.P2P.NetworkFingerprint = genesis
	runtimeCfg.StatePath = target
	runtimeCfg.InitialState = chain.Genesis(state.NetworkID)
	importedRuntime, err := New(runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := importedRuntime.Start(context.Background()); err != nil {
		t.Fatal("start imported NORMAL runtime:", err)
	}
	defer importedRuntime.Stop(context.Background())
	if runtimeHash(t, importedRuntime) != wantHash.String() {
		t.Fatal("started imported runtime has the wrong StateHash")
	}
	researchEntries, err := importedRuntime.ResearchList(0, 10)
	if err != nil || len(researchEntries.([]ResearchView)) != len(state.Research) {
		t.Fatalf("imported Research registry differs: %v", err)
	}
	resourceEntries, err := importedRuntime.ResourceList(0, 10)
	if err != nil || len(resourceEntries.([]ResourceView)) != len(state.Resources) {
		t.Fatalf("imported Resource registry differs: %v", err)
	}
	if count, _, _ := importedRuntime.ObjectStore().Usage(); count != 0 {
		t.Fatal("canonical state import incorrectly recreated off-chain objects")
	}
	feed, err := importedRuntime.BoardFeed(0, 10)
	if err != nil || len(feed.([]index.BoardEntry)) != 0 {
		t.Fatalf("canonical state import incorrectly recreated Board objects: %v", err)
	}
	if _, err := ImportState(exportPath, target, state.NetworkID, genesis); err == nil {
		t.Fatal("import silently overwrote existing state")
	}
	wrongGenesis := protocol.HashBytes([]byte("wrong"))
	if _, err := ImportState(exportPath, filepath.Join(directory, "wrong-genesis"), state.NetworkID, wrongGenesis); err == nil {
		t.Fatal("wrong genesis import succeeded")
	}
	if _, err := ImportState(exportPath, filepath.Join(directory, "wrong-network"), "another-network", genesis); err == nil {
		t.Fatal("wrong network import succeeded")
	}
}

func TestStateExportRejectsCorruptionAndMigratesPreviousSchema(t *testing.T) {
	directory := t.TempDir()
	genesis := protocol.HashBytes([]byte("phase-13-migration-genesis"))
	v3 := registryRegistryState(t)
	v2 := v3
	v2.SchemaVersion = chain.StateSchemaV2
	v2.Research = nil
	v2.Resources = nil
	canonical, err := v2.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := v2.Hash()
	envelope := StateExport{FormatVersion: StateExportFormatVersion, NetworkID: v2.NetworkID,
		GenesisAlgorithm: genesis.Algorithm, GenesisHex: strings.Repeat("00", 0) + bytesToHex(genesis.Digest),
		StateSchemaVersion: v2.SchemaVersion, AcceptedHeight: 7, StateHash: hash.String(), CanonicalState: canonical}
	path := filepath.Join(directory, "v2.json")
	writeExportFixture(t, path, envelope)
	target := filepath.Join(directory, "migrated", "state.snapshot")
	info, err := ImportState(path, target, v2.NetworkID, genesis)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Migrated || info.StateSchemaVersion != chain.StateSchemaV3 || info.StateHash == hash.String() {
		t.Fatalf("previous schema was not explicitly migrated: %+v", info)
	}
	loaded, err := mustStateStore(t, target, v2.NetworkID, genesis).Load()
	if err != nil || loaded.State.SchemaVersion != chain.StateSchemaV3 {
		t.Fatalf("migrated state was not durable: %v", err)
	}

	validData, _ := os.ReadFile(path)
	mutations := []func(*StateExport){
		func(value *StateExport) { value.FormatVersion++ },
		func(value *StateExport) { value.StateHash = "zion:state:sha256:" + strings.Repeat("00", 32) },
		func(value *StateExport) { value.StateSchemaVersion++ },
		func(value *StateExport) { value.CanonicalState[len(value.CanonicalState)-1] ^= 0xff },
	}
	for i, mutate := range mutations {
		var candidate StateExport
		if err := json.Unmarshal(validData, &candidate); err != nil {
			t.Fatal(err)
		}
		candidate.CanonicalState = append([]byte(nil), candidate.CanonicalState...)
		mutate(&candidate)
		corrupt := filepath.Join(directory, "corrupt-"+string(rune('a'+i))+".json")
		writeExportFixture(t, corrupt, candidate)
		if _, _, _, err := InspectStateExport(corrupt); err == nil {
			t.Fatalf("corrupt export %d was accepted", i)
		}
	}
	truncated := filepath.Join(directory, "truncated.json")
	if err := os.WriteFile(truncated, validData[:len(validData)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := InspectStateExport(truncated); err == nil {
		t.Fatal("truncated export was accepted")
	}
	oversized := filepath.Join(directory, "oversized.json")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte("x"), MaxStateExportBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := InspectStateExport(oversized); err == nil {
		t.Fatal("oversized export was accepted")
	}
	random := filepath.Join(directory, "random.json")
	if err := os.WriteFile(random, []byte("not an export"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := InspectStateExport(random); err == nil {
		t.Fatal("random export bytes were accepted")
	}
	v1 := chain.Genesis(protocol.Alpha1NetworkID)
	v1Bytes, _ := v1.CanonicalBytes()
	v1Hash, _ := v1.Hash()
	v1Path := filepath.Join(directory, "v1.json")
	writeExportFixture(t, v1Path, StateExport{FormatVersion: StateExportFormatVersion, NetworkID: v1.NetworkID,
		GenesisAlgorithm: genesis.Algorithm, GenesisHex: bytesToHex(genesis.Digest), StateSchemaVersion: v1.SchemaVersion,
		AcceptedHeight: 0, StateHash: v1Hash.String(), CanonicalState: v1Bytes})
	if _, err := ImportState(v1Path, filepath.Join(directory, "v1-target"), v1.NetworkID, genesis); err == nil {
		t.Fatal("state schema V1 was silently reinterpreted")
	}
}

func TestStateExportContainsNoSecretFields(t *testing.T) {
	directory := t.TempDir()
	genesis := protocol.HashBytes([]byte("secret-exclusion"))
	state := registryRegistryState(t)
	store := mustStateStore(t, filepath.Join(directory, "state"), state.NetworkID, genesis)
	if err := store.Save(state, 1); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "export.json")
	if _, err := ExportState(filepath.Join(directory, "state"), path, state.NetworkID, genesis); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, forbidden := range []string{"private_key", "seed", "bearer", "peer.key", "validator_key"} {
		if bytes.Contains(bytes.ToLower(data), []byte(forbidden)) {
			t.Fatalf("export contains forbidden secret field %q", forbidden)
		}
	}
}

func mustStateStore(t testing.TB, path string, network protocol.NetworkID, genesis protocol.HashDigest) *StateStore {
	t.Helper()
	store, err := NewStateStore(path, network, genesis)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeExportFixture(t testing.TB, path string, value StateExport) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func bytesToHex(value []byte) string {
	const digits = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for i, b := range value {
		result[i*2], result[i*2+1] = digits[b>>4], digits[b&15]
	}
	return string(result)
}
