// Package genesisfreeze provides the offline operator workflow for the frozen
// zion-alpha-1 shared genesis. It delegates all chain identity construction to
// internal/consensus and never implements a second GenesisID algorithm.
package genesisfreeze

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/privval"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/protocol"
)

const (
	ManifestSchema  = 1
	maxManifestSize = 64 * 1024
	maxGenesisSize  = 1024 * 1024
)

type Manifest struct {
	SchemaVersion int                `json:"schema_version"`
	NetworkID     protocol.NetworkID `json:"network_id"`
	GenesisTime   string             `json:"genesis_time"`
	Validators    []PublicValidator  `json:"validators"`
}

type PublicValidator struct {
	Name      string    `json:"name"`
	PublicKey PublicKey `json:"public_key"`
	Power     int64     `json:"power"`
}

type PublicKey struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type BuildResult struct {
	Genesis      consensus.Genesis
	GenesisBytes []byte
	Manifest     Manifest
}

type Verification struct {
	NetworkID                string   `json:"network_id"`
	GenesisID                string   `json:"genesis_id"`
	ChainID                  string   `json:"chain_id"`
	GenesisTime              string   `json:"genesis_time"`
	GenesisTimeIdentityBound bool     `json:"genesis_time_identity_bound"`
	Validators               int      `json:"validators"`
	VotingPower              int64    `json:"voting_power"`
	EqualVotingPower         bool     `json:"equal_voting_power"`
	ValidatorFingerprints    []string `json:"validator_fingerprints"`
	Validation               string   `json:"validation"`
}

func LoadManifest(path string) (Manifest, error) {
	raw, err := readRegular(path, maxManifestSize)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode public validator manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Manifest{}, fmt.Errorf("public validator manifest has trailing data")
	}
	if _, _, err := manifestInputs(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Build(manifest Manifest) (BuildResult, error) {
	validators, genesisTime, err := manifestInputs(manifest)
	if err != nil {
		return BuildResult{}, err
	}
	genesis, err := consensus.NewGenesis(manifest.NetworkID, validators)
	if err != nil {
		return BuildResult{}, err
	}
	document := genesis.CometGenesis(genesisTime)
	raw, err := cmtjson.MarshalIndent(document, "", "  ")
	if err != nil {
		return BuildResult{}, fmt.Errorf("encode CometBFT genesis: %w", err)
	}
	return BuildResult{Genesis: genesis, GenesisBytes: raw, Manifest: canonicalManifest(manifest, genesis.Validators)}, nil
}

func BuildFiles(manifestPath, genesisPath, genesisIDPath string) (BuildResult, error) {
	for _, path := range []string{genesisPath, genesisIDPath} {
		if _, err := os.Lstat(path); err == nil {
			return BuildResult{}, fmt.Errorf("refusing to overwrite existing file %s", filepath.Clean(path))
		} else if !os.IsNotExist(err) {
			return BuildResult{}, err
		}
	}
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return BuildResult{}, err
	}
	result, err := Build(manifest)
	if err != nil {
		return BuildResult{}, err
	}
	if err := writeExclusive(genesisPath, result.GenesisBytes, 0o644); err != nil {
		return BuildResult{}, err
	}
	id := hex.EncodeToString(result.Genesis.GenesisID.Digest) + "\n"
	if err := writeExclusive(genesisIDPath, []byte(id), 0o644); err != nil {
		return BuildResult{}, err
	}
	return result, nil
}

func VerifyFiles(manifestPath, genesisPath, genesisIDPath string) (Verification, error) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return Verification{}, err
	}
	expected, err := Build(manifest)
	if err != nil {
		return Verification{}, err
	}
	actualBytes, err := readRegular(genesisPath, maxGenesisSize)
	if err != nil {
		return Verification{}, err
	}
	if !bytes.Equal(actualBytes, expected.GenesisBytes) {
		return Verification{}, fmt.Errorf("committed genesis bytes differ from deterministic build")
	}
	loaded, err := consensus.LoadGenesis(genesisPath, manifest.NetworkID)
	if err != nil {
		return Verification{}, fmt.Errorf("verify consensus genesis: %w", err)
	}
	if !bytes.Equal(loaded.GenesisID.Digest, expected.Genesis.GenesisID.Digest) {
		return Verification{}, fmt.Errorf("loaded GenesisID differs from deterministic build")
	}
	idRaw, err := readRegular(genesisIDPath, 128)
	if err != nil {
		return Verification{}, err
	}
	wantID := hex.EncodeToString(expected.Genesis.GenesisID.Digest)
	if string(idRaw) != wantID+"\n" {
		return Verification{}, fmt.Errorf("genesis-id.txt must contain exactly the lowercase GenesisID and one newline")
	}
	fingerprints := make([]string, len(expected.Genesis.Validators))
	for i, validator := range expected.Genesis.Validators {
		fingerprints[i] = Fingerprint(validator.PublicKey)
	}
	return Verification{NetworkID: string(manifest.NetworkID), GenesisID: wantID, ChainID: expected.Genesis.ChainID,
		GenesisTime: manifest.GenesisTime, GenesisTimeIdentityBound: false, Validators: len(expected.Genesis.Validators),
		VotingPower: consensus.DefaultValidatorPower, EqualVotingPower: true,
		ValidatorFingerprints: fingerprints, Validation: "PASS"}, nil
}

// GenerateOperatorKeys creates exactly four fresh CometBFT private-validator
// key/state pairs using CometBFT's crypto/rand-backed Ed25519 generator. The
// destination and manifest must not exist; persistent identities are never
// overwritten. Only public keys are returned in the manifest.
func GenerateOperatorKeys(directory, manifestPath, genesisTime string) (Manifest, error) {
	if _, err := os.Lstat(directory); err == nil {
		return Manifest{}, fmt.Errorf("operator validator directory already exists")
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	}
	if _, err := os.Lstat(manifestPath); err == nil {
		return Manifest{}, fmt.Errorf("public validator manifest already exists")
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	}
	manifest := Manifest{SchemaVersion: ManifestSchema, NetworkID: protocol.Alpha1NetworkID, GenesisTime: genesisTime}
	if _, err := parseGenesisTime(genesisTime); err != nil {
		return Manifest{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Manifest{}, err
	}
	_ = os.Chmod(directory, 0o700)
	for i := 0; i < consensus.DefaultValidatorCount; i++ {
		name := fmt.Sprintf("validator-%d", i+1)
		root := filepath.Join(directory, name)
		configDirectory := filepath.Join(root, "config")
		dataDirectory := filepath.Join(root, "data")
		for _, path := range []string{root, configDirectory, dataDirectory} {
			if err := os.Mkdir(path, 0o700); err != nil {
				return Manifest{}, err
			}
			_ = os.Chmod(path, 0o700)
		}
		privateKey := cmted25519.GenPrivKey()
		keyPath := filepath.Join(configDirectory, "priv_validator_key.json")
		statePath := filepath.Join(dataDirectory, "priv_validator_state.json")
		filePV := privval.NewFilePV(privateKey, keyPath, statePath)
		keyBytes, err := cmtjson.MarshalIndent(filePV.Key, "", "  ")
		if err != nil {
			return Manifest{}, fmt.Errorf("encode validator key: %w", err)
		}
		stateBytes, err := cmtjson.MarshalIndent(filePV.LastSignState, "", "  ")
		if err != nil {
			return Manifest{}, fmt.Errorf("encode validator state: %w", err)
		}
		if err := writeExclusive(keyPath, keyBytes, 0o600); err != nil {
			return Manifest{}, err
		}
		if err := writeExclusive(statePath, stateBytes, 0o600); err != nil {
			return Manifest{}, err
		}
		manifest.Validators = append(manifest.Validators, PublicValidator{Name: name,
			PublicKey: PublicKey{Algorithm: "ed25519", Value: base64.StdEncoding.EncodeToString(privateKey.PubKey().Bytes())},
			Power:     consensus.DefaultValidatorPower})
	}
	sort.Slice(manifest.Validators, func(i, j int) bool {
		left, _ := base64.StdEncoding.DecodeString(manifest.Validators[i].PublicKey.Value)
		right, _ := base64.StdEncoding.DecodeString(manifest.Validators[j].PublicKey.Value)
		return bytes.Compare(left, right) < 0
	})
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := writeExclusive(manifestPath, manifestBytes, 0o644); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Fingerprint(publicKey []byte) string {
	digest := sha256.Sum256(publicKey)
	return hex.EncodeToString(digest[:6])
}

func manifestInputs(manifest Manifest) ([]consensus.Validator, time.Time, error) {
	if manifest.SchemaVersion != ManifestSchema || manifest.NetworkID != protocol.Alpha1NetworkID {
		return nil, time.Time{}, fmt.Errorf("manifest must describe schema %d network %s", ManifestSchema, protocol.Alpha1NetworkID)
	}
	genesisTime, err := parseGenesisTime(manifest.GenesisTime)
	if err != nil {
		return nil, time.Time{}, err
	}
	validators := make([]consensus.Validator, len(manifest.Validators))
	for i, item := range manifest.Validators {
		if item.PublicKey.Algorithm != "ed25519" || strings.TrimSpace(item.Name) != item.Name || item.Name == "" {
			return nil, time.Time{}, fmt.Errorf("invalid public validator %d", i)
		}
		key, err := base64.StdEncoding.Strict().DecodeString(item.PublicKey.Value)
		if err != nil || base64.StdEncoding.EncodeToString(key) != item.PublicKey.Value {
			return nil, time.Time{}, fmt.Errorf("invalid validator %q public key encoding", item.Name)
		}
		validators[i] = consensus.Validator{Name: item.Name, PublicKey: key, Power: item.Power}
	}
	// NewGenesis is the authoritative validator count/key/power/duplicate and
	// ordering validator. Calling it here keeps manifest validation identical.
	if _, err := consensus.NewGenesis(manifest.NetworkID, validators); err != nil {
		return nil, time.Time{}, err
	}
	return validators, genesisTime, nil
}

func canonicalManifest(manifest Manifest, validators []consensus.Validator) Manifest {
	byKey := make(map[string]PublicValidator, len(manifest.Validators))
	for _, validator := range manifest.Validators {
		byKey[validator.PublicKey.Value] = validator
	}
	result := manifest
	result.Validators = make([]PublicValidator, len(validators))
	for i, validator := range validators {
		value := base64.StdEncoding.EncodeToString(validator.PublicKey)
		result.Validators[i] = byKey[value]
	}
	return result
}

func parseGenesisTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || value != parsed.UTC().Format("2006-01-02T15:04:05Z") {
		return time.Time{}, fmt.Errorf("genesis_time must be an exact UTC whole-second timestamp")
	}
	return parsed, nil
}

func readRegular(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maximum {
		return nil, fmt.Errorf("input must be a non-empty regular file no larger than %d bytes", maximum)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read bounded input: %w", err)
	}
	if int64(len(raw)) > maximum {
		return nil, fmt.Errorf("input exceeds %d bytes", maximum)
	}
	return raw, nil
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("refusing to overwrite existing file %s", clean)
		}
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(clean)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_ = os.Chmod(clean, mode)
	ok = true
	return nil
}
