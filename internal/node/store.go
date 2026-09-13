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
	StorageSchemaVersion = 1
	MaxSnapshotBytes     = 8 * 1024 * 1024
	maxEnvelopeBytes     = MaxSnapshotBytes + 1024
)

var ErrSnapshotNotFound = errors.New("application snapshot not found")

type storedSnapshot struct {
	StorageVersion   uint64             `json:"storage_version"`
	NetworkID        protocol.NetworkID `json:"network_id"`
	GenesisAlgorithm string             `json:"genesis_algorithm"`
	GenesisHex       string             `json:"genesis_hex"`
	Height           int64              `json:"accepted_height"`
	StateHash        string             `json:"state_hash"`
	CanonicalState   []byte             `json:"canonical_state"`
}

type PersistedState struct {
	State  chain.State
	Height int64
}

type StateStore struct {
	path      string
	network   protocol.NetworkID
	genesisID protocol.HashDigest
}

func NewStateStore(path string, network protocol.NetworkID, genesisID protocol.HashDigest) (*StateStore, error) {
	if path == "" || network == "" || genesisID.Validate() != nil {
		return nil, fmt.Errorf("invalid application state store configuration")
	}
	return &StateStore{path: path, network: network,
		genesisID: protocol.HashDigest{Algorithm: genesisID.Algorithm, Digest: append([]byte(nil), genesisID.Digest...)}}, nil
}

func (s *StateStore) Load() (PersistedState, error) {
	state, err := s.loadPath(s.path)
	if errors.Is(err, os.ErrNotExist) {
		backup := s.path + ".previous"
		state, backupErr := s.loadPath(backup)
		if backupErr == nil {
			if renameErr := os.Rename(backup, s.path); renameErr != nil {
				return PersistedState{}, fmt.Errorf("recover interrupted application snapshot: %w", renameErr)
			}
			return state, nil
		}
		if errors.Is(backupErr, os.ErrNotExist) {
			return PersistedState{}, ErrSnapshotNotFound
		}
		return PersistedState{}, fmt.Errorf("verify recovery application snapshot: %w", backupErr)
	}
	if err != nil {
		return PersistedState{}, err
	}
	return state, nil
}

func (s *StateStore) loadPath(path string) (PersistedState, error) {
	data, err := readBounded(path, maxEnvelopeBytes)
	if err != nil {
		return PersistedState{}, fmt.Errorf("read application snapshot: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var envelope storedSnapshot
	if err := decoder.Decode(&envelope); err != nil {
		return PersistedState{}, fmt.Errorf("decode application snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return PersistedState{}, fmt.Errorf("application snapshot has trailing data")
	}
	return s.verify(envelope)
}

func (s *StateStore) verify(envelope storedSnapshot) (PersistedState, error) {
	if envelope.StorageVersion != StorageSchemaVersion || envelope.Height < 0 {
		return PersistedState{}, fmt.Errorf("unsupported application snapshot metadata")
	}
	if envelope.NetworkID != s.network {
		return PersistedState{}, fmt.Errorf("application snapshot network mismatch")
	}
	if envelope.GenesisAlgorithm != string(s.genesisID.Algorithm) || envelope.GenesisHex != hex.EncodeToString(s.genesisID.Digest) {
		return PersistedState{}, fmt.Errorf("application snapshot genesis mismatch")
	}
	if len(envelope.CanonicalState) == 0 || len(envelope.CanonicalState) > MaxSnapshotBytes {
		return PersistedState{}, fmt.Errorf("application snapshot payload exceeds limit")
	}
	var snapshot chain.Snapshot
	if err := protocol.CanonicalDecode(envelope.CanonicalState, &snapshot); err != nil {
		return PersistedState{}, fmt.Errorf("decode canonical state: %w", err)
	}
	state, err := chain.StateFromSnapshot(snapshot)
	if err != nil {
		return PersistedState{}, err
	}
	canonical, err := state.CanonicalBytes()
	if err != nil || !bytes.Equal(canonical, envelope.CanonicalState) {
		return PersistedState{}, fmt.Errorf("persisted state is not canonical")
	}
	hash, err := state.Hash()
	if err != nil || hash.String() != envelope.StateHash {
		return PersistedState{}, fmt.Errorf("application snapshot StateHash mismatch")
	}
	return PersistedState{State: state, Height: envelope.Height}, nil
}

func (s *StateStore) Save(state chain.State, height int64) error {
	if height < 0 || state.NetworkID != s.network {
		return fmt.Errorf("invalid application snapshot state or height")
	}
	canonical, err := state.CanonicalBytes()
	if err != nil {
		return err
	}
	if len(canonical) > MaxSnapshotBytes {
		return fmt.Errorf("canonical application state exceeds limit")
	}
	hash, err := state.Hash()
	if err != nil {
		return err
	}
	envelope := storedSnapshot{StorageVersion: StorageSchemaVersion, NetworkID: s.network,
		GenesisAlgorithm: string(s.genesisID.Algorithm), GenesisHex: hex.EncodeToString(s.genesisID.Digest), Height: height,
		StateHash: hash.String(), CanonicalState: canonical}
	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode application snapshot: %w", err)
	}
	if len(data) > maxEnvelopeBytes {
		return fmt.Errorf("encoded application snapshot exceeds limit")
	}
	return replaceFile(s.path, data, 0o600)
}

func readBounded(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("file exceeds %d-byte limit", maximum)
	}
	data := make([]byte, info.Size())
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, err
	}
	return data, nil
}

func replaceFile(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".state-*")
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
	backup := path + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
