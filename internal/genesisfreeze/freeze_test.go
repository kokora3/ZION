package genesisfreeze

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/protocol"
)

const (
	frozenGenesisID       = "72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6"
	frozenGenesisFileHash = "d6eec9bed4ee0b6118a1f40c4f48399127c5112387f409fa7b107c3eed84b720"
	localOnlyGenesisID    = "4cce10c9eb93aa5baff6ec94b13ff27662464668764a39efed6d59373a487d55"
	testKeyNotice         = "TEST ONLY - PUBLIC FIXTURE - NOT SECRET - NEVER USE IN PRODUCTION"
)

func sharedPaths() (string, string, string) {
	root := filepath.Join("..", "..", "configs", "alpha-1")
	return filepath.Join(root, "validators-public.json"), filepath.Join(root, "genesis.json"), filepath.Join(root, "genesis-id.txt")
}

func TestSharedGenesisFrozenAndReproducible(t *testing.T) {
	manifestPath, genesisPath, idPath := sharedPaths()
	verified, err := VerifyFiles(manifestPath, genesisPath, idPath)
	if err != nil {
		t.Fatal(err)
	}
	if verified.GenesisID != frozenGenesisID || verified.NetworkID != "zion-alpha-1" || verified.Validators != 4 || !verified.EqualVotingPower {
		t.Fatalf("unexpected frozen genesis verification: %+v", verified)
	}
	genesisBytes, err := os.ReadFile(genesisPath)
	if err != nil {
		t.Fatal(err)
	}
	fileHash := sha256.Sum256(genesisBytes)
	if hex.EncodeToString(fileHash[:]) != frozenGenesisFileHash {
		t.Fatalf("frozen genesis file SHA-256 changed: %x", fileHash)
	}
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Build(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for replay := 0; replay < 100; replay++ {
		next, err := Build(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first.GenesisBytes, next.GenesisBytes) ||
			!bytes.Equal(first.Genesis.GenesisID.Digest, next.Genesis.GenesisID.Digest) {
			t.Fatalf("genesis replay %d was nondeterministic", replay+1)
		}
	}

	reversed := manifest
	reversed.Validators = append([]PublicValidator(nil), manifest.Validators...)
	for left, right := 0, len(reversed.Validators)-1; left < right; left, right = left+1, right-1 {
		reversed.Validators[left], reversed.Validators[right] = reversed.Validators[right], reversed.Validators[left]
	}
	reordered, err := Build(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.GenesisBytes, reordered.GenesisBytes) {
		t.Fatal("validator allocation/insertion order changed canonical genesis bytes")
	}
}

func TestSharedGenesisRejectsInvalidValidatorInputs(t *testing.T) {
	manifestPath, _, _ := sharedPaths()
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"wrong count", func(m *Manifest) { m.Validators = m.Validators[:3] }},
		{"duplicate key", func(m *Manifest) { m.Validators[3].PublicKey = m.Validators[0].PublicKey }},
		{"duplicate name", func(m *Manifest) { m.Validators[3].Name = m.Validators[0].Name }},
		{"wrong power", func(m *Manifest) { m.Validators[0].Power = 2 }},
		{"zero power", func(m *Manifest) { m.Validators[0].Power = 0 }},
		{"wrong algorithm", func(m *Manifest) { m.Validators[0].PublicKey.Algorithm = "secp256k1" }},
		{"malformed encoding", func(m *Manifest) { m.Validators[0].PublicKey.Value = "not-base64" }},
		{"wrong key length", func(m *Manifest) { m.Validators[0].PublicKey.Value = base64.StdEncoding.EncodeToString([]byte{1}) }},
		{"wrong network", func(m *Manifest) { m.NetworkID = "zion-alpha-2" }},
		{"non UTC timestamp", func(m *Manifest) { m.GenesisTime = "2026-09-14T07:00:00+07:00" }},
		{"fractional timestamp", func(m *Manifest) { m.GenesisTime = "2026-09-14T00:00:00.001Z" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := manifest
			candidate.Validators = append([]PublicValidator(nil), manifest.Validators...)
			test.mutate(&candidate)
			if _, err := Build(candidate); err == nil {
				t.Fatal("invalid shared genesis input accepted")
			}
		})
	}
}

func TestSharedGenesisIdentityMutationAndSeparation(t *testing.T) {
	manifestPath, genesisPath, _ := sharedPaths()
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	original, err := Build(manifest)
	if err != nil {
		t.Fatal(err)
	}
	validators, _, err := manifestInputs(manifest)
	if err != nil {
		t.Fatal(err)
	}
	invalidVersionState := chain.Genesis(manifest.NetworkID)
	invalidVersionState.ProtocolVersion = protocol.ProtocolVersion("0.2")
	if _, err := consensus.NewGenesisWithState(manifest.NetworkID, validators, invalidVersionState); err == nil {
		t.Fatal("unsupported initial protocol version was accepted")
	}
	mutated := manifest
	mutated.Validators = append([]PublicValidator(nil), manifest.Validators...)
	key, _ := base64.StdEncoding.DecodeString(mutated.Validators[0].PublicKey.Value)
	key[0] ^= 0x01
	mutated.Validators[0].PublicKey.Value = base64.StdEncoding.EncodeToString(key)
	changed, err := Build(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(original.Genesis.GenesisID.Digest, changed.Genesis.GenesisID.Digest) {
		t.Fatal("validator public-key mutation did not change GenesisID")
	}

	timestampOnly := manifest
	timestampOnly.GenesisTime = "2026-09-16T00:00:00Z"
	changedTime, err := Build(timestampOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original.Genesis.GenesisID.Digest, changedTime.Genesis.GenesisID.Digest) {
		t.Fatal("CometBFT operational genesis timestamp unexpectedly changed ZION GenesisID")
	}
	if bytes.Equal(original.GenesisBytes, changedTime.GenesisBytes) {
		t.Fatal("timestamp mutation did not change frozen CometBFT genesis bytes")
	}

	raw, err := os.ReadFile(genesisPath)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name string
		old  string
		new  string
	}{
		{"application schema", `"schema_version": 1`, `"schema_version": 2`},
		{"application network", `"network_id": "zion-alpha-1"`, `"network_id": "zion-alpha-2"`},
		{"application state", "52b47af40a8068309bcd792303d8afb1b09ee3687a930d45d9692d64ed7c0505", "42b47af40a8068309bcd792303d8afb1b09ee3687a930d45d9692d64ed7c0505"},
		{"validator-set hash", "168f8207d5dd8e766bc82c6f0fee652467a8a463a24f60fdee1e3d6850dd872c", "068f8207d5dd8e766bc82c6f0fee652467a8a463a24f60fdee1e3d6850dd872c"},
		{"chain identity", "zion-alpha-1-72ef0c7816d6", "zion-alpha-1-62ef0c7816d6"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutatedApp := bytes.Replace(raw, []byte(mutation.old), []byte(mutation.new), 1)
			if bytes.Equal(mutatedApp, raw) {
				t.Fatal("mutation target not found")
			}
			path := filepath.Join(t.TempDir(), "genesis.json")
			if err := os.WriteFile(path, mutatedApp, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := consensus.LoadGenesis(path, manifest.NetworkID); err == nil {
				t.Fatal("identity-bound genesis mutation was accepted")
			}
		})
	}
	if frozenGenesisID == localOnlyGenesisID {
		t.Fatal("shared genesis equals documented local-only genesis")
	}
}

func TestSharedValidatorsDoNotReuseConsensusTestKeys(t *testing.T) {
	manifestPath, _, _ := sharedPaths()
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	testKeys := make(map[string]struct{}, consensus.DefaultValidatorCount)
	for i := 0; i < consensus.DefaultValidatorCount; i++ {
		key := cmted25519.GenPrivKeyFromSecret([]byte(testKeyNotice + string(rune('A'+i))))
		testKeys[base64.StdEncoding.EncodeToString(key.PubKey().Bytes())] = struct{}{}
	}
	for _, validator := range manifest.Validators {
		if _, exists := testKeys[validator.PublicKey.Value]; exists {
			t.Fatalf("shared validator %q reuses a Phase 5 test key", validator.Name)
		}
	}
	testValidators := make([]consensus.Validator, 0, len(testKeys))
	for i := 0; i < consensus.DefaultValidatorCount; i++ {
		key := cmted25519.GenPrivKeyFromSecret([]byte(testKeyNotice + string(rune('A'+i))))
		testValidators = append(testValidators, consensus.Validator{Name: string(rune('A' + i)), PublicKey: key.PubKey().Bytes(), Power: 1})
	}
	testGenesis, err := consensus.NewGenesis(manifest.NetworkID, testValidators)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(testGenesis.GenesisID.Digest) == frozenGenesisID {
		t.Fatal("shared GenesisID equals Phase 5 test GenesisID")
	}
}

func TestOperatorKeyGenerationIsExclusiveAndPublicManifestIsSecretFree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "validators")
	manifestPath := filepath.Join(t.TempDir(), "validators-public.json")
	manifest, err := GenerateOperatorKeys(root, manifestPath, "2026-09-15T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Validators) != consensus.DefaultValidatorCount {
		t.Fatalf("generated validators=%d", len(manifest.Validators))
	}
	publicBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicBytes), `"priv_key"`) || strings.Contains(strings.ToLower(string(publicBytes)), "seed") {
		t.Fatal("public manifest contains private validator material")
	}
	if _, err := GenerateOperatorKeys(root, filepath.Join(t.TempDir(), "second.json"), "2026-09-15T00:00:00Z"); err == nil {
		t.Fatal("existing operator key directory was overwritten")
	}
}
