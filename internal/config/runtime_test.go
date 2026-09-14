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
	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
)

func TestAllAlphaExampleConfigsParseStrictly(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "configs", "alpha-1", "*.yaml"))
	if err != nil || len(paths) < 5 {
		t.Fatalf("expected alpha configuration profiles: %v (%d)", err, len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			file, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if file.NetworkID != string(protocol.Alpha1NetworkID) || file.GenesisID == "" || file.DataDirectory == "" {
				t.Fatal("example omits required network, genesis, or data-directory field")
			}
			if _, err := ParseLogLevel(file.LogLevel); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestD1DistributionConfigsAreStrictAndSafe(t *testing.T) {
	root := filepath.Join("..", "..")
	portable := []struct {
		name  string
		roles []p2p.Role
	}{
		{name: "normal.yaml", roles: []p2p.Role{p2p.RoleNormal}},
		{name: "bootstrap.yaml", roles: []p2p.Role{p2p.RoleNormal, p2p.RoleBootstrap}},
	}
	for _, profile := range portable {
		t.Run(profile.name, func(t *testing.T) {
			file, err := Load(filepath.Join(root, "configs", "alpha-1", profile.name))
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := file.RuntimeConfig()
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.P2P.Roles) != len(profile.roles) {
				t.Fatalf("roles = %v", cfg.P2P.Roles)
			}
			for index := range profile.roles {
				if cfg.P2P.Roles[index] != profile.roles[index] {
					t.Fatalf("roles = %v", cfg.P2P.Roles)
				}
			}
			if cfg.ConsensusFactory != nil || cfg.FreshStateIsAuthoritative {
				t.Fatal("non-validator distribution config enabled consensus authority")
			}
		})
	}

	strictOnly := []string{
		filepath.Join(root, "configs", "alpha-1", "validator.yaml.example"),
		filepath.Join(root, "deploy", "compose", "configs", "normal.yaml"),
		filepath.Join(root, "deploy", "compose", "configs", "bootstrap.yaml"),
		filepath.Join(root, "deploy", "compose", "configs", "validator.yaml.example"),
	}
	for _, path := range strictOnly {
		if _, err := Load(path); err != nil {
			t.Fatalf("strict load %s: %v", path, err)
		}
	}
}

func TestRuntimeConfigNormalAndValidatorComposition(t *testing.T) {
	fingerprint := protocol.HashBytes([]byte("phase-8-config-normal"))
	normal := File{NetworkID: string(protocol.Alpha1NetworkID), GenesisID: hex.EncodeToString(fingerprint.Digest),
		DataDirectory: t.TempDir(), Roles: []p2p.Role{p2p.RoleNormal}, LogLevel: "WARN",
		P2P:     P2P{AdvertiseAddresses: []string{"/dns4/bootstrap.public.test/udp/42000/quic-v1"}},
		API:     API{MetricsEnabled: boolPointer(false)},
		Objects: Objects{Directory: filepath.Join(t.TempDir(), "object-store"), QuotaBytes: 8 << 20},
		Board: Board{IndexPath: filepath.Join(t.TempDir(), "board-index.json"), AnnounceFanout: 4, SyncInterval: "2s",
			SyncPageSize: 8, MaxSyncEventsPerCycle: 64, MaxConcurrentHandlers: 3, PeerTimeout: "1s"}}
	normalConfig, err := normal.RuntimeConfig()
	if err != nil || normalConfig.ConsensusFactory != nil || normalConfig.FreshStateIsAuthoritative {
		t.Fatalf("normal runtime config: %v", err)
	}
	if normalConfig.ObjectDirectory != normal.Objects.Directory || normalConfig.ObjectQuotaBytes != normal.Objects.QuotaBytes {
		t.Fatal("local object-store configuration was not applied")
	}
	if normalConfig.API.MetricsEnabled {
		t.Fatal("metrics_enabled configuration was not applied")
	}
	if len(normalConfig.P2P.AdvertiseAddresses) != 1 || normalConfig.P2P.AdvertiseAddresses[0] != normal.P2P.AdvertiseAddresses[0] {
		t.Fatal("advertise_addresses configuration was not applied")
	}
	if normalConfig.BoardIndexPath != normal.Board.IndexPath || normalConfig.Board.AnnounceFanout != 4 ||
		normalConfig.Board.SyncInterval != 2*time.Second || normalConfig.Board.SyncPageSize != 8 ||
		normalConfig.Board.MaxSyncEventsPerCycle != 64 || normalConfig.Board.MaxConcurrentHandlers != 3 ||
		normalConfig.Board.PeerTimeout != time.Second || normalConfig.Board.Enabled != board.DefaultConfig().Enabled {
		t.Fatal("local Board configuration was not applied")
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

func boolPointer(value bool) *bool { return &value }

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
