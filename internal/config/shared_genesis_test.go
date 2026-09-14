package config

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/node"
	"github.com/kokora3/zion/internal/p2p"
)

func TestSharedAlphaNormalConfigConsumesFrozenGenesis(t *testing.T) {
	configPath := filepath.Join("..", "..", "configs", "alpha-1", "normal.example.yaml")
	genesisPath := filepath.Join("..", "..", "configs", "alpha-1", "genesis.json")
	file, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	genesis, err := consensus.LoadGenesis(genesisPath, "zion-alpha-1")
	if err != nil {
		t.Fatal(err)
	}
	if file.GenesisID != hex.EncodeToString(genesis.GenesisID.Digest) {
		t.Fatalf("shared NORMAL config GenesisID=%q, want frozen genesis", file.GenesisID)
	}

	// Make the committed shared template local and isolated without changing its
	// network identity, then prove the production runtime accepts and starts it.
	directory := t.TempDir()
	enabled := true
	file.DataDirectory = directory
	file.Roles = []p2p.Role{p2p.RoleNormal}
	file.P2P.Enabled = &enabled
	file.P2P.ListenAddresses = nil
	file.P2P.BootstrapAddresses = nil
	file.P2P.FallbackAddresses = nil
	file.P2P.ManualPeers = nil
	file.P2P.PeerKeyPath = filepath.Join(directory, "p2p", "peer.key")
	file.P2P.PeerCachePath = filepath.Join(directory, "p2p", "peers.json")
	file.API.Listen = "127.0.0.1:0"
	file.API.AllowedOrigins = nil
	file.Objects.Directory = filepath.Join(directory, "objects")
	file.Board.IndexPath = filepath.Join(directory, "board", "index.json")
	file.Registries.IndexPath = filepath.Join(directory, "registries", "index.json")
	file.Consensus = Consensus{}
	runtimeConfig, err := file.RuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := node.New(runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
