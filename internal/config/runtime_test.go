package config

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	cmtconfig "github.com/cometbft/cometbft/config"
	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmtp2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
)

func TestRuntimeConfigNormalAndValidatorComposition(t *testing.T) {
	fingerprint := protocol.HashBytes([]byte("phase-8-config-normal"))
	normal := File{NetworkID: string(protocol.Alpha1NetworkID), GenesisID: hex.EncodeToString(fingerprint.Digest),
		DataDirectory: t.TempDir(), Roles: []p2p.Role{p2p.RoleNormal},
		Objects: Objects{Directory: filepath.Join(t.TempDir(), "object-store"), QuotaBytes: 8 << 20}}
	normalConfig, err := normal.RuntimeConfig()
	if err != nil || normalConfig.ConsensusFactory != nil || normalConfig.FreshStateIsAuthoritative {
		t.Fatalf("normal runtime config: %v", err)
	}
	if normalConfig.ObjectDirectory != normal.Objects.Directory || normalConfig.ObjectQuotaBytes != normal.Objects.QuotaBytes {
		t.Fatal("local object-store configuration was not applied")
	}

	root := filepath.Join(t.TempDir(), "consensus")
	comet := cmtconfig.DefaultConfig().SetRoot(root)
	privateKeys := make([]cmted25519.PrivKey, consensus.DefaultValidatorCount)
	validators := make([]consensus.Validator, consensus.DefaultValidatorCount)
	for index := range validators {
		privateKeys[index] = cmted25519.GenPrivKeyFromSecret([]byte("TEST ONLY PUBLIC CONFIG VALIDATOR " + string(rune('A'+index))))
		validators[index] = consensus.Validator{Name: "validator-" + string(rune('a'+index)),
			PublicKey: privateKeys[index].PubKey().Bytes(), Power: consensus.DefaultValidatorPower}
	}
	genesis, err := consensus.NewGenesis(protocol.Alpha1NetworkID, validators)
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{filepath.Dir(comet.GenesisFile()), filepath.Dir(comet.PrivValidatorStateFile())} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := genesis.CometGenesis(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)).SaveAs(comet.GenesisFile()); err != nil {
		t.Fatal(err)
	}
	pv := privval.NewFilePV(privateKeys[0], comet.PrivValidatorKeyFile(), comet.PrivValidatorStateFile())
	pv.Save()
	nodeKey := &cmtp2p.NodeKey{PrivKey: cmted25519.GenPrivKeyFromSecret([]byte("TEST ONLY PUBLIC CONFIG NODE KEY"))}
	if err := nodeKey.SaveAs(comet.NodeKeyFile()); err != nil {
		t.Fatal(err)
	}
	validatorFile := File{NetworkID: string(protocol.Alpha1NetworkID), GenesisID: hex.EncodeToString(genesis.GenesisID.Digest),
		DataDirectory: filepath.Join(t.TempDir(), "zion"), Roles: []p2p.Role{p2p.RoleNormal, p2p.RoleValidator},
		Consensus: Consensus{Enabled: true, RootDirectory: root, P2PListenAddress: "tcp://127.0.0.1:0"}}
	validatorConfig, err := validatorFile.RuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if validatorConfig.ConsensusFactory == nil || !validatorConfig.FreshStateIsAuthoritative ||
		len(validatorConfig.ValidatorPublicKey) != len(privateKeys[0].PubKey().Bytes()) {
		t.Fatal("validator runtime was not composed from verified CometBFT files")
	}
	wrong := validatorFile
	wrong.GenesisID = hex.EncodeToString(protocol.HashBytes([]byte("wrong")).Digest)
	if _, err := wrong.RuntimeConfig(); err == nil {
		t.Fatal("mismatched consensus genesis accepted")
	}
	withoutRole := validatorFile
	withoutRole.Roles = []p2p.Role{p2p.RoleNormal}
	if _, err := withoutRole.RuntimeConfig(); err == nil {
		t.Fatal("consensus enabled without VALIDATOR role")
	}
}

func TestLoadRejectsUnknownConfigurationField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zion.yaml")
	data := []byte("network_id: zion-alpha-1\ngenesis_id: " +
		hex.EncodeToString(protocol.HashBytes([]byte("config")).Digest) + "\ndata_directory: data\nunknown: true\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("unknown configuration field accepted")
	}
}
