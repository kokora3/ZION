package p2p

import (
	"context"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/protocol"
	libp2p "github.com/libp2p/go-libp2p"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
)

func firstFullAddress(t testing.TB, node *Node) string {
	t.Helper()
	addresses := node.FullAddresses()
	if len(addresses) == 0 {
		t.Fatal("node has no reachable address")
	}
	return addresses[0].String()
}

func waitUsable(t testing.TB, node *Node, peerID libpeer.ID, expected bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if node.IsUsable(peerID) == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("peer %s usable=%v, want %v", peerID, node.IsUsable(peerID), expected)
}

func TestBootstrapDiscoveryDisappearanceAndCacheRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	bootstrapCfg := testConfig(t, "bootstrap", true)
	bootstrapCfg.Roles = []Role{RoleBootstrap, RoleNormal}
	bootstrap, err := NewNode(ctx, bootstrapCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Close()

	peerCCfg := testConfig(t, "peer-c", true)
	peerC, err := NewNode(ctx, peerCCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer peerC.Close()
	bootstrapAddress := firstFullAddress(t, bootstrap)
	if err := peerC.Dial(ctx, bootstrapAddress, SourceBootstrap); err != nil {
		t.Fatal(err)
	}
	waitUsable(t, bootstrap, peerC.PeerID(), true)

	peerACfg := testConfig(t, "peer-a-outbound-only", false)
	peerACfg.BootstrapAddresses = []string{bootstrapAddress}
	peerACfg.Limits.TargetOutboundPeers = 2
	peerA, err := NewNode(ctx, peerACfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(peerA.ListenAddresses()) != 0 {
		t.Fatal("outbound-only normal node unexpectedly listens")
	}
	peerAID := peerA.PeerID()
	if err := peerA.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	waitUsable(t, peerA, bootstrap.PeerID(), true)
	waitUsable(t, peerA, peerC.PeerID(), true)
	waitUsable(t, peerC, peerA.PeerID(), true)

	if err := bootstrap.Close(); err != nil {
		t.Fatal(err)
	}
	waitUsable(t, peerA, peerC.PeerID(), true)
	waitUsable(t, peerC, peerA.PeerID(), true)

	if err := peerA.Close(); err != nil {
		t.Fatal(err)
	}
	restartCfg := peerACfg
	restartCfg.BootstrapAddresses = nil
	restartedA, err := NewNode(ctx, restartCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedA.Close()
	if restartedA.PeerID() != peerAID {
		t.Fatal("persisted P2P key did not retain PeerID")
	}
	if err := restartedA.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	waitUsable(t, restartedA, peerC.PeerID(), true)
}

func TestHelloCompatibilityMatrixAndWrongExpectedPeerID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverCfg := testConfig(t, "compat-server", true)
	server, err := NewNode(ctx, serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	serverAddress := firstFullAddress(t, server)

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"wrong-network", func(c *Config) { c.NetworkID = "another-network" }},
		{"wrong-genesis", func(c *Config) { c.NetworkFingerprint = fixtureFingerprint("different-genesis") }},
		{"incompatible-version", func(c *Config) { c.SupportedVersions = []Version{{Major: 1}} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clientCfg := testConfig(t, test.name, false)
			test.mutate(&clientCfg)
			client, err := NewNode(ctx, clientCfg)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := client.Dial(ctx, serverAddress, SourceManual); err == nil {
				t.Fatal("incompatible peer accepted")
			}
			if client.IsUsable(server.PeerID()) || server.IsUsable(client.PeerID()) {
				t.Fatal("incompatible peer promoted to usable set")
			}
		})
	}

	wrongID := writeTestKey(t, filepath.Join(t.TempDir(), "wrong.key"), "wrong-expected-id")
	badAddress := strings.Replace(serverAddress, server.PeerID().String(), wrongID.String(), 1)
	clientCfg := testConfig(t, "wrong-id-client", false)
	client, err := NewNode(ctx, clientCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Dial(ctx, badAddress, SourceManual); err == nil {
		t.Fatal("reachable address with wrong expected PeerID authenticated")
	}
	if len(client.UsablePeers()) != 0 {
		t.Fatal("wrong PeerID entered usable set")
	}
	if len(client.cache.SuccessfulCandidates(client.PeerID())) != 0 {
		t.Fatal("failed PeerID authentication entered successful cache")
	}
}

func TestWrongNetworkPEXHintStillRequiresHello(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wrongCfg := testConfig(t, "wrong-network-pex-target", true)
	wrongCfg.NetworkID = "another-network"
	wrong, err := NewNode(ctx, wrongCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	clientCfg := testConfig(t, "pex-hint-client", false)
	client, err := NewNode(ctx, clientCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	record := PeerRecord{PeerID: wrong.PeerID().String(), Addresses: sortedAddressStrings(wrong.ListenAddresses(), 8), Roles: []Role{RoleNormal}}
	client.dialPEXHints(ctx, []PeerRecord{record})
	if client.IsUsable(wrong.PeerID()) {
		t.Fatal("wrong-network peer learned through PEX bypassed hello")
	}
}

func TestDuplicateDialsProduceOneLogicalPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server, err := NewNode(ctx, testConfig(t, "duplicate-server", true))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := NewNode(ctx, testConfig(t, "duplicate-client", false))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	address := firstFullAddress(t, server)
	errors := make(chan error, 8)
	for i := 0; i < cap(errors); i++ {
		go func() { errors <- client.Dial(ctx, address, SourceManual) }()
	}
	for i := 0; i < cap(errors); i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	if peers := client.UsablePeers(); len(peers) != 1 || peers[0].PeerID != server.PeerID() {
		t.Fatalf("duplicate connections created duplicate logical peers: %+v", peers)
	}
}

func TestMalformedAndOversizedHelloNeverPromoted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server, err := NewNode(ctx, testConfig(t, "malformed-hello-server", true))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	addressInfo, err := parseDialAddress(firstFullAddress(t, server))
	if err != nil {
		t.Fatal(err)
	}
	for _, oversized := range []bool{false, true} {
		raw, err := libp2p.New(libp2p.NoListenAddrs, libp2p.Transport(quic.NewTransport), libp2p.DisableRelay(), libp2p.DisableMetrics())
		if err != nil {
			t.Fatal(err)
		}
		if err := raw.Connect(ctx, *addressInfo); err != nil {
			_ = raw.Close()
			t.Fatal(err)
		}
		stream, err := raw.NewStream(ctx, server.PeerID(), HelloProtocolID)
		if err != nil {
			_ = raw.Close()
			t.Fatal(err)
		}
		if oversized {
			var prefix [4]byte
			binary.BigEndian.PutUint32(prefix[:], MaxHelloSize+1)
			_, _ = stream.Write(prefix[:])
		} else {
			_ = writeFrame(stream, []byte{0xff}, MaxHelloSize)
		}
		_ = stream.Close()
		time.Sleep(25 * time.Millisecond)
		if server.IsUsable(raw.ID()) {
			_ = raw.Close()
			t.Fatal("malformed or oversized hello entered usable peer set")
		}
		_ = raw.Close()
	}
}

func TestDiscoveryFallbackManualDNSAndMultipleBootstrap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	serverCfg := testConfig(t, "discovery-server", true)
	server, err := NewNode(ctx, serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	address := firstFullAddress(t, server)
	unreachable := strings.Replace(address, "/udp/", "/udp/1/p2p/", 1)
	// The replacement above contains two /p2p components; use a valid closed port instead.
	info, _ := parseDialAddress(address)
	unreachable = fmt.Sprintf("/ip4/127.0.0.1/udp/1/quic-v1/p2p/%s", info.ID)

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"multiple-bootstrap", func(c *Config) { c.BootstrapAddresses = []string{unreachable, address} }},
		{"static-fallback", func(c *Config) { c.BootstrapAddresses = []string{unreachable}; c.FallbackAddresses = []string{address} }},
		{"manual", func(c *Config) {
			c.BootstrapAddresses = []string{unreachable}
			c.FallbackAddresses = []string{unreachable}
			c.ManualPeers = []string{address}
		}},
		{"dns", func(c *Config) {
			c.Limits.DialTimeout = 2 * time.Second
			c.BootstrapAddresses = []string{strings.Replace(address, "/ip4/127.0.0.1/", "/dns4/localhost/", 1)}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig(t, test.name, false)
			cfg.Limits.DialTimeout = 200 * time.Millisecond
			test.mutate(&cfg)
			client, err := NewNode(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := client.Discover(ctx); err != nil {
				t.Fatal(err)
			}
			waitUsable(t, client, server.PeerID(), true)
		})
	}
}

func TestRepeatedStartStopWithPersistedPeerID(t *testing.T) {
	directory := t.TempDir()
	cfg := DefaultConfig(directory, fixtureFingerprint("alpha-genesis"))
	cfg.ListenAddresses = []string{"/ip4/127.0.0.1/udp/0/quic-v1"}
	cfg.Limits = DefaultLimits()
	var expected libpeer.ID
	for i := 0; i < 5; i++ {
		node, err := NewNode(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			expected = node.PeerID()
		} else if node.PeerID() != expected {
			t.Fatal("PeerID changed across repeated start/stop")
		}
		if err := node.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestP2PConfigurationCannotChangeCanonicalState(t *testing.T) {
	state := chainStateHash(t)
	cfgA := testConfig(t, "isolation-a", false)
	cfgB := testConfig(t, "isolation-b", true)
	cfgB.Roles = []Role{RoleBootstrap, RoleNormal, RoleValidator}
	bootstrapID := writeTestKey(t, filepath.Join(t.TempDir(), "bootstrap.key"), "isolation-bootstrap")
	cfgB.BootstrapAddresses = []string{"/dns4/example.invalid/udp/42000/quic-v1/p2p/" + bootstrapID.String()}
	nodeA, err := NewNode(context.Background(), cfgA)
	if err != nil {
		t.Fatal(err)
	}
	if err := nodeA.Close(); err != nil {
		t.Fatal(err)
	}
	nodeB, err := NewNode(context.Background(), cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if err := nodeB.Close(); err != nil {
		t.Fatal(err)
	}
	if state != chainStateHash(t) {
		t.Fatal("P2P operational configuration changed canonical state")
	}
}

func chainStateHash(t testing.TB) string {
	t.Helper()
	state := chain.Genesis(protocol.Alpha1NetworkID)
	hash, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return hash.String()
}
