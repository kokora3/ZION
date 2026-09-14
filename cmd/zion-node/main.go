package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kokora3/zion/internal/buildinfo"
	localconfig "github.com/kokora3/zion/internal/config"
	"github.com/kokora3/zion/internal/node"
	"github.com/kokora3/zion/internal/p2p"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintln(os.Stdout, buildinfo.String("zion-node"))
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "state" {
		stateCommand(os.Args[2:])
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "config" {
		configCommand(os.Args[2:])
		return
	}

	if len(os.Args) < 2 || os.Args[1] != "run" {
		fmt.Fprintln(os.Stderr, "usage: zion-node run --config <zion.yaml> | zion-node config validate | zion-node state export|inspect|import | zion-node version")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := flags.String("config", "zion.yaml", "node configuration file")
	dataDir := flags.String("data-dir", "", "override local data directory")
	apiListen := flags.String("api-listen", "", "override local API address")
	p2pListen := flags.String("p2p-listen", "", "override general P2P multiaddr")
	p2pAdvertise := flags.String("p2p-advertise", "", "override the public P2P multiaddr advertised to peers")
	manualPeer := flags.String("manual-peer", "", "append one manual peer multiaddr")
	_ = flags.Parse(os.Args[2:])

	file, err := localconfig.Load(*configPath)
	if err != nil {
		fail(err)
	}
	if *dataDir != "" {
		file.DataDirectory = *dataDir
	}
	cfg, err := file.RuntimeConfig()
	if err != nil {
		fail(err)
	}
	logLevel, err := localconfig.ParseLogLevel(file.LogLevel)
	if err != nil {
		fail(err)
	}
	cfg.Logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	if *apiListen != "" {
		cfg.API.Listen = *apiListen
	}
	if *p2pListen != "" {
		cfg.P2P.ListenAddresses = []string{*p2pListen}
	}
	if *p2pAdvertise != "" {
		cfg.P2P.AdvertiseAddresses = []string{*p2pAdvertise}
	}
	if *manualPeer != "" {
		cfg.P2P.ManualPeers = append(cfg.P2P.ManualPeers, *manualPeer)
	}
	if host, _, splitErr := net.SplitHostPort(cfg.API.Listen); splitErr == nil {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			fmt.Fprintln(os.Stderr, "WARNING: ZION API is explicitly exposed beyond loopback; bearer authentication is required")
		}
	}
	runtime, err := node.New(cfg)
	if err != nil {
		fail(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		fail(err)
	}
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer stop()
	if err := runtime.Stop(shutdown); err != nil {
		fail(err)
	}
}

func configCommand(args []string) {
	if len(args) < 1 || args[0] != "validate" {
		fail(fmt.Errorf("usage: zion-node config validate --config <zion.yaml> [--p2p-advertise <multiaddr>] [--expected-network <id>] [--expected-genesis-id <hex>]"))
	}
	flags := flag.NewFlagSet("config validate", flag.ExitOnError)
	configPath := flags.String("config", "zion.yaml", "node configuration file")
	p2pAdvertise := flags.String("p2p-advertise", "", "override the public P2P multiaddr advertised to peers")
	expectedNetwork := flags.String("expected-network", "", "require this NetworkID")
	expectedGenesisID := flags.String("expected-genesis-id", "", "require this lowercase GenesisID")
	_ = flags.Parse(args[1:])
	if flags.NArg() != 0 {
		fail(fmt.Errorf("unexpected config validate arguments"))
	}
	file, err := localconfig.Load(*configPath)
	if err != nil {
		fail(err)
	}
	cfg, err := file.RuntimeConfig()
	if err != nil {
		fail(err)
	}
	if *p2pAdvertise != "" {
		cfg.P2P.AdvertiseAddresses = []string{*p2pAdvertise}
	}
	if err := node.ValidateConfig(cfg); err != nil {
		fail(err)
	}
	if err := cfg.P2P.Validate(); err != nil {
		fail(fmt.Errorf("invalid P2P configuration: %w", err))
	}
	if *expectedNetwork != "" && string(cfg.NetworkID) != *expectedNetwork {
		fail(fmt.Errorf("configured NetworkID %q differs from expected %q", cfg.NetworkID, *expectedNetwork))
	}
	genesisID := fmt.Sprintf("%x", cfg.GenesisID.Digest)
	if *expectedGenesisID != "" && genesisID != *expectedGenesisID {
		fail(fmt.Errorf("configured GenesisID %q differs from expected %q", genesisID, *expectedGenesisID))
	}
	writeJSON(map[string]any{"valid": true, "network_id": cfg.NetworkID,
		"genesis_id": genesisID, "roles": cfg.P2P.Roles,
		"p2p_advertise_addresses": cfg.P2P.AdvertiseAddresses})
}

func stateCommand(args []string) {
	if len(args) < 1 {
		fail(fmt.Errorf("usage: zion-node state export|inspect|import"))
	}
	switch args[0] {
	case "inspect":
		flags := flag.NewFlagSet("state inspect", flag.ExitOnError)
		input := flags.String("in", "", "state export file")
		_ = flags.Parse(args[1:])
		if *input == "" || flags.NArg() != 0 {
			fail(fmt.Errorf("usage: zion-node state inspect --in <export.json>"))
		}
		info, _, _, err := node.InspectStateExport(*input)
		if err != nil {
			fail(err)
		}
		writeJSON(info)
	case "export", "import":
		flags := flag.NewFlagSet("state "+args[0], flag.ExitOnError)
		configPath := flags.String("config", "zion.yaml", "node configuration file")
		input := flags.String("in", "", "state export file")
		output := flags.String("out", "", "new state export file")
		_ = flags.Parse(args[1:])
		if flags.NArg() != 0 {
			fail(fmt.Errorf("unexpected state command arguments"))
		}
		file, err := localconfig.Load(*configPath)
		if err != nil {
			fail(err)
		}
		if args[0] == "import" {
			if file.Consensus.Enabled || containsValidatorRole(file.Roles) {
				fail(fmt.Errorf("state import is restricted to a stopped, fresh non-validator node"))
			}
		}
		cfg, err := file.RuntimeConfig()
		if err != nil {
			fail(err)
		}
		var info node.StateExportInfo
		if args[0] == "export" {
			if *output == "" || *input != "" {
				fail(fmt.Errorf("usage: zion-node state export --config <zion.yaml> --out <new-export.json>"))
			}
			info, err = node.ExportState(cfg.StatePath, *output, cfg.NetworkID, cfg.GenesisID)
		} else {
			if *input == "" || *output != "" {
				fail(fmt.Errorf("usage: zion-node state import --config <zion.yaml> --in <export.json>"))
			}
			info, err = node.ImportState(*input, cfg.StatePath, cfg.NetworkID, cfg.GenesisID)
		}
		if err != nil {
			fail(err)
		}
		writeJSON(info)
	default:
		fail(fmt.Errorf("unknown state command %q", args[0]))
	}
}

func containsValidatorRole(roles []p2p.Role) bool {
	for _, role := range roles {
		if role == p2p.RoleValidator {
			return true
		}
	}
	return false
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "zion-node:", err)
	time.Sleep(10 * time.Millisecond)
	os.Exit(1)
}
