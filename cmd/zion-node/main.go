package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	localconfig "github.com/kokora3/zion/internal/config"
	"github.com/kokora3/zion/internal/node"
	"github.com/kokora3/zion/internal/protocol"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintln(os.Stdout, protocol.VersionString("zion-node"))
		return
	}

	if len(os.Args) < 2 || os.Args[1] != "run" {
		fmt.Fprintln(os.Stderr, "usage: zion-node run --config <zion.yaml> | zion-node version")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := flags.String("config", "zion.yaml", "node configuration file")
	dataDir := flags.String("data-dir", "", "override local data directory")
	apiListen := flags.String("api-listen", "", "override local API address")
	p2pListen := flags.String("p2p-listen", "", "override general P2P multiaddr")
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
	if *apiListen != "" {
		cfg.API.Listen = *apiListen
	}
	if *p2pListen != "" {
		cfg.P2P.ListenAddresses = []string{*p2pListen}
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

func fail(err error) {
	fmt.Fprintln(os.Stderr, "zion-node:", err)
	time.Sleep(10 * time.Millisecond)
	os.Exit(1)
}
