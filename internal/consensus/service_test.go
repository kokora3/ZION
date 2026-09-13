package consensus

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cmtconfig "github.com/cometbft/cometbft/config"
	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/kokora3/zion/internal/protocol"
)

func TestServiceRejectsInvalidValidatorFileWithoutProcessExit(t *testing.T) {
	validators := make([]Validator, DefaultValidatorCount)
	for index := range validators {
		key := cmted25519.GenPrivKeyFromSecret([]byte("TEST ONLY PUBLIC INVALID SERVICE " + string(rune('A'+index))))
		validators[index] = Validator{Name: "validator-" + string(rune('a'+index)), PublicKey: key.PubKey().Bytes(), Power: DefaultValidatorPower}
	}
	genesis, err := NewGenesis(protocol.Alpha1NetworkID, validators)
	if err != nil {
		t.Fatal(err)
	}
	application, err := NewApplication(genesis)
	if err != nil {
		t.Fatal(err)
	}
	cfg := cmtconfig.TestConfig().SetRoot(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(cfg.PrivValidatorKeyFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.PrivValidatorKeyFile(), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(cfg, application)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Start(context.Background()); err == nil {
		t.Fatal("invalid validator key file was accepted")
	}
	if service.Active() {
		t.Fatal("failed validator service remained active")
	}
}
