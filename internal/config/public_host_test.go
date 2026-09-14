package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
)

func TestD2APublicHostConfigUsesFrozenIdentityAndNoAuthority(t *testing.T) {
	root := filepath.Join("..", "..")
	path := filepath.Join(root, "deploy", "public-host", "bootstrap.yaml")
	file, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	genesisBytes, err := os.ReadFile(filepath.Join(root, "configs", "alpha-1", "genesis-id.txt"))
	if err != nil {
		t.Fatal(err)
	}
	wantGenesis := strings.TrimSpace(string(genesisBytes))
	if file.NetworkID != string(protocol.Alpha1NetworkID) || file.GenesisID != wantGenesis {
		t.Fatalf("public identity = %s/%s, want %s/%s", file.NetworkID, file.GenesisID, protocol.Alpha1NetworkID, wantGenesis)
	}
	if len(file.Roles) != 2 || file.Roles[0] != p2p.RoleNormal || file.Roles[1] != p2p.RoleBootstrap {
		t.Fatalf("public roles = %v", file.Roles)
	}
	if file.Consensus.Enabled || file.Consensus.RootDirectory != "" {
		t.Fatal("public bootstrap configuration enabled validator consensus")
	}
	if file.API.Listen != "0.0.0.0:42001" || file.API.BearerTokenFile == "" {
		t.Fatal("container-local API must retain bearer authentication")
	}
	if len(file.P2P.ListenAddresses) != 1 || file.P2P.ListenAddresses[0] != "/ip4/0.0.0.0/udp/42000/quic-v1" {
		t.Fatalf("public P2P listener = %v", file.P2P.ListenAddresses)
	}
	file.API.BearerTokenFile = ""
	file.P2P.AdvertiseAddresses = []string{"/dns4/bootstrap.public.test/udp/42000/quic-v1"}
	cfg, err := file.RuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.P2P.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.ConsensusFactory != nil || cfg.FreshStateIsAuthoritative {
		t.Fatal("NORMAL + BOOTSTRAP configuration acquired validator authority")
	}
}
