package node

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/protocol"
)

const (
	StateExportFormatVersion = 1
	MaxStateExportBytes      = MaxSnapshotBytes + 4096
)

// StateExport is the explicit, portable and secret-free canonical state
// envelope. CanonicalState contains exactly the chain Snapshot CBOR bytes.
type StateExport struct {
	FormatVersion      uint64                 `json:"format_version"`
	NetworkID          protocol.NetworkID     `json:"network_id"`
	GenesisAlgorithm   protocol.HashAlgorithm `json:"genesis_algorithm"`
	GenesisHex         string                 `json:"genesis_hex"`
	StateSchemaVersion protocol.SchemaVersion `json:"state_schema_version"`
	AcceptedHeight     int64                  `json:"accepted_height"`
	StateHash          string                 `json:"state_hash"`
	CanonicalState     []byte                 `json:"canonical_state"`
}

type StateExportInfo struct {
	FormatVersion      uint64                 `json:"format_version"`
	NetworkID          protocol.NetworkID     `json:"network_id"`
	GenesisID          string                 `json:"genesis_id"`
	StateSchemaVersion protocol.SchemaVersion `json:"state_schema_version"`
	AcceptedHeight     int64                  `json:"accepted_height"`
	StateHash          string                 `json:"state_hash"`
	Migrated           bool                   `json:"migrated,omitempty"`
}

// ExportState verifies the durable snapshot before writing a new export file.
// The output path is never silently overwritten.
func ExportState(statePath, outputPath string, network protocol.NetworkID, genesis protocol.HashDigest) (StateExportInfo, error) {
	store, err := NewStateStore(statePath, network, genesis)
	if err != nil {
		return StateExportInfo{}, err
	}
	persisted, err := store.Load()
	if err != nil {
		return StateExportInfo{}, fmt.Errorf("load state for export: %w", err)
	}
	canonical, err := persisted.State.CanonicalBytes()
	if err != nil {
		return StateExportInfo{}, err
	}
	hash, err := persisted.State.Hash()
	if err != nil {
		return StateExportInfo{}, err
	}
	envelope := StateExport{FormatVersion: StateExportFormatVersion, NetworkID: network,
		GenesisAlgorithm: genesis.Algorithm, GenesisHex: hex.EncodeToString(genesis.Digest),
		StateSchemaVersion: persisted.State.SchemaVersion, AcceptedHeight: persisted.Height,
		StateHash: hash.String(), CanonicalState: canonical}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return StateExportInfo{}, err
	}
	data = append(data, '\n')
	if len(data) > MaxStateExportBytes {
		return StateExportInfo{}, fmt.Errorf("state export exceeds %d-byte limit", MaxStateExportBytes)
	}
	if err := writeNewAtomic(outputPath, data, 0o600); err != nil {
		return StateExportInfo{}, fmt.Errorf("write state export: %w", err)
	}
	return exportInfo(envelope, false), nil
}

// InspectStateExport performs the same bounded integrity checks used by import.
func InspectStateExport(path string) (StateExportInfo, PersistedState, protocol.HashDigest, error) {
	data, err := readBounded(path, MaxStateExportBytes)
	if err != nil {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("read state export: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var envelope StateExport
	if err := decoder.Decode(&envelope); err != nil {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("decode state export: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("state export has trailing data")
	}
	if envelope.FormatVersion != StateExportFormatVersion || envelope.AcceptedHeight < 0 {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("unsupported state export metadata")
	}
	genesisBytes, err := hex.DecodeString(envelope.GenesisHex)
	if err != nil || hex.EncodeToString(genesisBytes) != envelope.GenesisHex {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("invalid state export genesis")
	}
	genesis := protocol.HashDigest{Algorithm: envelope.GenesisAlgorithm, Digest: genesisBytes}
	if err := genesis.Validate(); err != nil {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("invalid state export genesis: %w", err)
	}
	if envelope.NetworkID == "" || len(envelope.CanonicalState) == 0 || len(envelope.CanonicalState) > MaxSnapshotBytes {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("invalid or oversized state export payload")
	}
	var snapshot chain.Snapshot
	if err := protocol.CanonicalDecode(envelope.CanonicalState, &snapshot); err != nil {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("decode exported canonical state: %w", err)
	}
	state, err := chain.StateFromSnapshot(snapshot)
	if err != nil {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("verify exported canonical state: %w", err)
	}
	canonical, err := state.CanonicalBytes()
	if err != nil || !bytes.Equal(canonical, envelope.CanonicalState) {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("exported state is not canonical")
	}
	hash, err := state.Hash()
	if err != nil || hash.String() != envelope.StateHash {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("state export StateHash mismatch")
	}
	if state.NetworkID != envelope.NetworkID || state.SchemaVersion != envelope.StateSchemaVersion {
		return StateExportInfo{}, PersistedState{}, protocol.HashDigest{}, fmt.Errorf("state export envelope differs from canonical state")
	}
	return exportInfo(envelope, false), PersistedState{State: state, Height: envelope.AcceptedHeight}, genesis, nil
}

// ImportState installs a verified export into a fresh state path. Schema V2 is
// migrated through the existing explicit deterministic V2 -> V3 migration.
// V1 lacks the governance bootstrap information needed for a safe migration.
func ImportState(inputPath, statePath string, network protocol.NetworkID, genesis protocol.HashDigest) (StateExportInfo, error) {
	info, persisted, exportedGenesis, err := InspectStateExport(inputPath)
	if err != nil {
		return StateExportInfo{}, err
	}
	if info.NetworkID != network {
		return StateExportInfo{}, fmt.Errorf("state import network mismatch")
	}
	if !equalHash(exportedGenesis, genesis) {
		return StateExportInfo{}, fmt.Errorf("state import genesis mismatch")
	}
	migrated := false
	switch persisted.State.SchemaVersion {
	case chain.StateSchemaV3:
	case chain.StateSchemaV2:
		persisted.State, err = chain.BootstrapRegistries(persisted.State)
		if err != nil {
			return StateExportInfo{}, fmt.Errorf("migrate state schema v2: %w", err)
		}
		migrated = true
	default:
		return StateExportInfo{}, fmt.Errorf("unsupported import state schema %d", persisted.State.SchemaVersion)
	}
	lock, err := acquireImportLock(statePath)
	if err != nil {
		return StateExportInfo{}, err
	}
	defer func() { _ = os.Remove(lock) }()
	if _, err := os.Stat(statePath); err == nil {
		return StateExportInfo{}, fmt.Errorf("refusing to overwrite existing application state")
	} else if !errors.Is(err, os.ErrNotExist) {
		return StateExportInfo{}, fmt.Errorf("inspect import destination: %w", err)
	}
	if _, err := os.Stat(statePath + ".previous"); err == nil {
		return StateExportInfo{}, fmt.Errorf("refusing import while recovery state exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return StateExportInfo{}, fmt.Errorf("inspect import recovery destination: %w", err)
	}
	store, err := NewStateStore(statePath, network, genesis)
	if err != nil {
		return StateExportInfo{}, err
	}
	if err := store.Save(persisted.State, persisted.Height); err != nil {
		return StateExportInfo{}, fmt.Errorf("install imported state: %w", err)
	}
	hash, _ := persisted.State.Hash()
	info.StateSchemaVersion = persisted.State.SchemaVersion
	info.StateHash = hash.String()
	info.Migrated = migrated
	return info, nil
}

func exportInfo(envelope StateExport, migrated bool) StateExportInfo {
	return StateExportInfo{FormatVersion: envelope.FormatVersion, NetworkID: envelope.NetworkID,
		GenesisID:          string(envelope.GenesisAlgorithm) + ":" + envelope.GenesisHex,
		StateSchemaVersion: envelope.StateSchemaVersion, AcceptedHeight: envelope.AcceptedHeight,
		StateHash: envelope.StateHash, Migrated: migrated}
}

func acquireImportLock(statePath string) (string, error) {
	if statePath == "" {
		return "", fmt.Errorf("empty import destination")
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return "", err
	}
	path := statePath + ".import.lock"
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("state import already in progress")
		}
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func writeNewAtomic(path string, data []byte, mode os.FileMode) error {
	if path == "" {
		return fmt.Errorf("empty output path")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".export-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	// A hard link publishes the complete same-volume temporary file while
	// preserving create-if-absent semantics on Windows and POSIX filesystems.
	if err := os.Link(name, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("destination already exists")
		}
		return err
	}
	_ = syncDirectory(directory)
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
