package consensus

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/privval"
)

// TestSharedGenesisWithOperatorKeys is an explicit local/manual acceptance
// test. Ordinary CI intentionally has no real alpha private keys and skips it.
func TestSharedGenesisWithOperatorKeys(t *testing.T) {
	operatorDirectory := os.Getenv("ZION_ALPHA_VALIDATOR_DIR")
	if operatorDirectory == "" {
		t.Skip("set ZION_ALPHA_VALIDATOR_DIR for the operator-only shared-genesis acceptance test")
	}
	genesisPath := filepath.Join("..", "..", "configs", "alpha-1", "genesis.json")
	genesis, err := LoadGenesis(genesisPath, "zion-alpha-1")
	if err != nil {
		t.Fatal(err)
	}
	genesisBytes, err := os.ReadFile(genesisPath)
	if err != nil {
		t.Fatal(err)
	}
	privateKeys := make([]cmted25519.PrivKey, 0, DefaultValidatorCount)
	for i := 1; i <= DefaultValidatorCount; i++ {
		root := filepath.Join(operatorDirectory, "validator-"+string(rune('0'+i)))
		filePV := privval.LoadFilePV(filepath.Join(root, "config", "priv_validator_key.json"),
			filepath.Join(root, "data", "priv_validator_state.json"))
		key, ok := filePV.Key.PrivKey.(cmted25519.PrivKey)
		if !ok {
			t.Fatalf("validator-%d does not contain an Ed25519 consensus key", i)
		}
		privateKeys = append(privateKeys, key)
	}
	for _, expected := range genesis.Validators {
		found := false
		for _, privateKey := range privateKeys {
			if bytes.Equal(expected.PublicKey, privateKey.PubKey().Bytes()) {
				found = true
			}
		}
		if !found {
			t.Fatalf("operator keys do not match frozen validator %q", expected.Name)
		}
	}

	network := newLocalNetworkWithGenesis(t, genesis, privateKeys, true)
	// Exercise the exact frozen public genesis bytes, including its operational
	// timestamp, rather than the integration helper's normal test timestamp.
	for _, validator := range network.nodes {
		if err := os.WriteFile(validator.config.GenesisFile(), genesisBytes, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	network.start(t, 0, 1, 2, 3)
	transaction := fixtureBytes(t, "identity_create_golden.json")
	network.submit(t, 0, transaction)
	statuses := network.waitConvergedState(t, []int{0, 1, 2, 3}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 1
	})
	requireStateConverged(t, statuses)
}
