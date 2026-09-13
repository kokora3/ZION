package config

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	cmtconfig "github.com/cometbft/cometbft/config"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/node"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
	"gopkg.in/yaml.v3"
)

type File struct {
	NetworkID     string     `yaml:"network_id"`
	GenesisID     string     `yaml:"genesis_id"`
	DataDirectory string     `yaml:"data_directory"`
	Roles         []p2p.Role `yaml:"roles"`
	P2P           P2P        `yaml:"p2p"`
	API           API        `yaml:"api"`
	Objects       Objects    `yaml:"objects"`
	Consensus     Consensus  `yaml:"consensus"`
}

type P2P struct {
	Enabled                *bool    `yaml:"enabled"`
	ListenAddresses        []string `yaml:"listen_addresses"`
	BootstrapAddresses     []string `yaml:"bootstrap_addresses"`
	FallbackAddresses      []string `yaml:"fallback_addresses"`
	ManualPeers            []string `yaml:"manual_peers"`
	PeerKeyPath            string   `yaml:"peer_key_path"`
	PeerCachePath          string   `yaml:"peer_cache_path"`
	MaxConnectedPeers      int      `yaml:"max_connected_peers"`
	TargetOutboundPeers    int      `yaml:"target_outbound_peers"`
	MaxConcurrentDials     int      `yaml:"max_concurrent_dials"`
	MaxCachedPeers         int      `yaml:"max_cached_peers"`
	MaxAddressesPerPeer    int      `yaml:"max_addresses_per_peer"`
	MaxPEXPeersPerResponse int      `yaml:"max_pex_peers_per_response"`
}

type API struct {
	Listen          string   `yaml:"listen"`
	AllowedOrigins  []string `yaml:"allowed_origins"`
	BearerTokenFile string   `yaml:"bearer_token_file"`
}

// Objects is local operational configuration and never canonical chain state.
type Objects struct {
	Directory  string `yaml:"directory"`
	QuotaBytes uint64 `yaml:"quota_bytes"`
}

type Consensus struct {
	Enabled           bool   `yaml:"enabled"`
	RootDirectory     string `yaml:"root_directory"`
	Moniker           string `yaml:"moniker"`
	P2PListenAddress  string `yaml:"p2p_listen_address"`
	PersistentPeers   string `yaml:"persistent_peers"`
	RPCListenAddress  string `yaml:"rpc_listen_address"`
	CreateEmptyBlocks *bool  `yaml:"create_empty_blocks"`
}

func Load(path string) (File, error) {
	file, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(io.LimitReader(file, 1024*1024))
	decoder.KnownFields(true)
	var result File
	if err := decoder.Decode(&result); err != nil {
		return File{}, fmt.Errorf("decode node configuration: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return File{}, fmt.Errorf("node configuration has trailing document")
	}
	return result, nil
}

func (f File) RuntimeConfig() (node.Config, error) {
	if f.NetworkID == "" {
		f.NetworkID = string(protocol.Alpha1NetworkID)
	}
	if f.NetworkID != string(protocol.Alpha1NetworkID) || f.DataDirectory == "" {
		return node.Config{}, fmt.Errorf("invalid network or data directory")
	}
	fingerprint, err := parseDigest(f.GenesisID)
	if err != nil {
		return node.Config{}, err
	}
	initial := chain.Genesis(protocol.NetworkID(f.NetworkID))
	cfg := node.DefaultConfig(f.DataDirectory, fingerprint, initial)
	cfg.NetworkID = protocol.NetworkID(f.NetworkID)
	if len(f.Roles) == 0 {
		f.Roles = []p2p.Role{p2p.RoleNormal}
	}
	cfg.P2P.Roles = append([]p2p.Role(nil), f.Roles...)
	if f.P2P.ListenAddresses != nil {
		cfg.P2P.ListenAddresses = append([]string(nil), f.P2P.ListenAddresses...)
	}
	cfg.P2P.BootstrapAddresses = append([]string(nil), f.P2P.BootstrapAddresses...)
	cfg.P2P.FallbackAddresses = append([]string(nil), f.P2P.FallbackAddresses...)
	cfg.P2P.ManualPeers = append([]string(nil), f.P2P.ManualPeers...)
	if f.P2P.Enabled != nil {
		cfg.P2P.Enabled = *f.P2P.Enabled
	}
	if f.P2P.PeerKeyPath != "" {
		cfg.P2P.KeyPath = f.P2P.PeerKeyPath
	}
	if f.P2P.PeerCachePath != "" {
		cfg.P2P.PeerCachePath = f.P2P.PeerCachePath
	}
	if f.P2P.MaxConnectedPeers != 0 {
		cfg.P2P.Limits.MaxConnectedPeers = f.P2P.MaxConnectedPeers
	}
	if f.P2P.TargetOutboundPeers != 0 {
		cfg.P2P.Limits.TargetOutboundPeers = f.P2P.TargetOutboundPeers
	}
	if f.P2P.MaxConcurrentDials != 0 {
		cfg.P2P.Limits.MaxConcurrentDials = f.P2P.MaxConcurrentDials
	}
	if f.P2P.MaxCachedPeers != 0 {
		cfg.P2P.Limits.MaxCachedPeers = f.P2P.MaxCachedPeers
	}
	if f.P2P.MaxAddressesPerPeer != 0 {
		cfg.P2P.Limits.MaxAddressesPerPeer = f.P2P.MaxAddressesPerPeer
	}
	if f.P2P.MaxPEXPeersPerResponse != 0 {
		cfg.P2P.Limits.MaxPEXPeersPerResponse = f.P2P.MaxPEXPeersPerResponse
	}
	if f.API.Listen != "" {
		cfg.API.Listen = f.API.Listen
	}
	if f.Objects.Directory != "" {
		cfg.ObjectDirectory = f.Objects.Directory
	}
	if f.Objects.QuotaBytes != 0 {
		cfg.ObjectQuotaBytes = f.Objects.QuotaBytes
	}
	cfg.API.AllowedOrigins = append([]string(nil), f.API.AllowedOrigins...)
	if f.API.BearerTokenFile != "" {
		data, err := os.ReadFile(filepath.Clean(f.API.BearerTokenFile))
		if err != nil {
			return node.Config{}, err
		}
		if len(data) < 16 || len(data) > 4096 {
			return node.Config{}, fmt.Errorf("invalid API bearer token file")
		}
		cfg.API.BearerToken = strings.TrimSpace(string(data))
		if len(cfg.API.BearerToken) < 16 {
			return node.Config{}, fmt.Errorf("invalid API bearer token file")
		}
	}
	cfg.ShutdownTimeout = 10 * time.Second
	hasValidatorRole := false
	for _, role := range f.Roles {
		if role == p2p.RoleValidator {
			hasValidatorRole = true
		}
	}
	if hasValidatorRole != f.Consensus.Enabled {
		return node.Config{}, fmt.Errorf("VALIDATOR role and consensus.enabled must be configured together")
	}
	if f.Consensus.Enabled {
		if f.Consensus.RootDirectory == "" {
			return node.Config{}, fmt.Errorf("consensus root_directory is required")
		}
		comet := cmtconfig.DefaultConfig().SetRoot(filepath.Clean(f.Consensus.RootDirectory))
		if f.Consensus.Moniker != "" {
			comet.Moniker = f.Consensus.Moniker
		}
		if f.Consensus.P2PListenAddress != "" {
			comet.P2P.ListenAddress = f.Consensus.P2PListenAddress
		}
		comet.P2P.PersistentPeers = f.Consensus.PersistentPeers
		comet.RPC.ListenAddress = f.Consensus.RPCListenAddress
		comet.GRPC.ListenAddress = ""
		if f.Consensus.CreateEmptyBlocks != nil {
			comet.Consensus.CreateEmptyBlocks = *f.Consensus.CreateEmptyBlocks
		}
		genesis, err := consensus.LoadGenesis(comet.GenesisFile(), cfg.NetworkID)
		if err != nil {
			return node.Config{}, fmt.Errorf("load consensus genesis: %w", err)
		}
		if !bytes.Equal(genesis.GenesisID.Digest, cfg.GenesisID.Digest) {
			return node.Config{}, fmt.Errorf("configured genesis_id differs from consensus genesis")
		}
		validatorKey, err := consensus.ValidatorPublicKey(comet.PrivValidatorKeyFile())
		if err != nil {
			return node.Config{}, err
		}
		cfg.ValidatorPublicKey = validatorKey
		for _, validator := range genesis.Validators {
			cfg.GenesisValidatorKeys = append(cfg.GenesisValidatorKeys, append([]byte(nil), validator.PublicKey...))
		}
		cfg.FreshStateIsAuthoritative = true
		cfg.ConsensusFactory = func(state chain.State, height int64) (node.ConsensusService, error) {
			application, err := consensus.NewApplicationFromState(genesis, state, height)
			if err != nil {
				return nil, err
			}
			return consensus.NewService(comet, application)
		}
	}
	return cfg, nil
}

func parseDigest(value string) (protocol.HashDigest, error) {
	if len(value) != 64 {
		return protocol.HashDigest{}, fmt.Errorf("genesis_id must be 64 lowercase SHA-256 hex characters")
	}
	digest, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(digest) != value {
		return protocol.HashDigest{}, fmt.Errorf("invalid genesis_id")
	}
	result := protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: digest}
	return result, result.Validate()
}
