package p2p

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	libpeer "github.com/libp2p/go-libp2p/core/peer"
)

func waitForMaintenanceAttempts(t testing.TB, node *Node, minimum uint64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if node.maintenanceAttempts.Load() >= minimum {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("maintenance attempts = %d, want at least %d", node.maintenanceAttempts.Load(), minimum)
}

func restartTestBootstrap(t testing.TB, ctx context.Context, cfg Config, expected libpeer.ID) *Node {
	t.Helper()
	node, err := NewNode(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if node.PeerID() != expected {
		_ = node.Close()
		t.Fatal("bootstrap PeerID changed across recreation")
	}
	return node
}

func TestOutboundOnlyNormalReconnectsWhenBootstrapReturns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bootstrapCfg := testConfig(t, "persistent-bootstrap", true)
	bootstrapCfg.Roles = []Role{RoleNormal, RoleBootstrap}
	bootstrap, err := NewNode(ctx, bootstrapCfg)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapID := bootstrap.PeerID()
	bootstrapAddress := firstFullAddress(t, bootstrap)
	bootstrapCfg.ListenAddresses = []string{strings.TrimSuffix(bootstrapAddress, "/p2p/"+bootstrapID.String())}
	bootstrapAddress = strings.Replace(bootstrapAddress, "/ip4/127.0.0.1/", "/dns4/localhost/", 1)

	normalCfg := testConfig(t, "persistent-outbound-normal", false)
	normalCfg.BootstrapAddresses = []string{bootstrapAddress}
	normalCfg.Limits.TargetOutboundPeers = 1
	normalCfg.Limits.DialTimeout = 50 * time.Millisecond
	normalCfg.Limits.HandshakeTimeout = 50 * time.Millisecond
	normal, err := NewNode(ctx, normalCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer normal.Close()
	normalID := normal.PeerID()
	if len(normal.ListenAddresses()) != 0 || len(normal.AdvertisedAddresses()) != 0 {
		t.Fatal("NORMAL is not outbound-only")
	}
	if err := normal.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	waitUsable(t, normal, bootstrapID, true)
	waitUsable(t, bootstrap, normalID, true)
	if connected, outbound := normal.ConnectionCounts(); connected != 1 || outbound != 1 {
		t.Fatalf("initial connection counts = %d/%d", connected, outbound)
	}

	if err := bootstrap.Close(); err != nil {
		t.Fatal(err)
	}
	waitUsable(t, normal, bootstrapID, false)
	if connected, outbound := normal.ConnectionCounts(); connected != 0 || outbound != 0 {
		t.Fatalf("disconnected counts = %d/%d", connected, outbound)
	}
	attemptsBeforeOutage := normal.maintenanceAttempts.Load()
	// Exercise more than one configured retry interval while the bootstrap is absent.
	time.Sleep(10 * normalCfg.Limits.BackoffMaximum)
	if attempts := normal.maintenanceAttempts.Load(); attempts < attemptsBeforeOutage+2 {
		t.Fatalf("long outage produced only %d maintenance attempts", attempts-attemptsBeforeOutage)
	}

	restarted := restartTestBootstrap(t, ctx, bootstrapCfg, bootstrapID)
	defer restarted.Close()

	// The NORMAL process deliberately remains running and receives no manual Discover call.
	waitUsable(t, normal, bootstrapID, true)
	waitUsable(t, restarted, normalID, true)
	if connected, outbound := normal.ConnectionCounts(); connected != 1 || outbound != 1 {
		t.Fatalf("reconnected counts = %d/%d", connected, outbound)
	}
	if normal.PeerID() != normalID {
		t.Fatal("running NORMAL PeerID changed during bootstrap recreation")
	}
}

func TestOutboundMaintenanceSurvivesRapidBootstrapFlaps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	bootstrapCfg := testConfig(t, "flapping-bootstrap", true)
	bootstrapCfg.Roles = []Role{RoleNormal, RoleBootstrap}
	bootstrap, err := NewNode(ctx, bootstrapCfg)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapID := bootstrap.PeerID()
	address := firstFullAddress(t, bootstrap)
	bootstrapCfg.ListenAddresses = []string{strings.TrimSuffix(address, "/p2p/"+bootstrapID.String())}

	normalCfg := testConfig(t, "flapping-normal", false)
	normalCfg.BootstrapAddresses = []string{address}
	normalCfg.Limits.TargetOutboundPeers = 1
	normalCfg.Limits.DialTimeout = 50 * time.Millisecond
	normalCfg.Limits.HandshakeTimeout = 50 * time.Millisecond
	normal, err := NewNode(ctx, normalCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer normal.Close()
	waitUsable(t, normal, bootstrapID, true)
	startAttempts := normal.maintenanceAttempts.Load()
	for range 3 {
		if err := bootstrap.Close(); err != nil {
			t.Fatal(err)
		}
		waitUsable(t, normal, bootstrapID, false)
		bootstrap = restartTestBootstrap(t, ctx, bootstrapCfg, bootstrapID)
		waitUsable(t, normal, bootstrapID, true)
	}
	defer bootstrap.Close()
	if attempts := normal.maintenanceAttempts.Load() - startAttempts; attempts > 18 {
		t.Fatalf("rapid flap caused a dial storm: %d maintenance attempts", attempts)
	}
}

func TestOutboundMaintenanceRejectsWrongExpectedPeerID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server, err := NewNode(ctx, testConfig(t, "wrong-id-server", true))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	actualAddress := firstFullAddress(t, server)
	wrongID := writeTestKey(t, filepath.Join(t.TempDir(), "expected.key"), "wrong-id-expected")
	configured := strings.Replace(actualAddress, server.PeerID().String(), wrongID.String(), 1)

	cfg := testConfig(t, "wrong-id-maintainer", false)
	cfg.BootstrapAddresses = []string{configured}
	cfg.Limits.TargetOutboundPeers = 1
	cfg.Limits.DialTimeout = 50 * time.Millisecond
	cfg.Limits.HandshakeTimeout = 50 * time.Millisecond
	client, err := NewNode(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	waitForMaintenanceAttempts(t, client, 2)
	if connected, outbound := client.ConnectionCounts(); connected != 0 || outbound != 0 {
		t.Fatalf("wrong PeerID became usable: %d/%d", connected, outbound)
	}
}

func TestOutboundMaintenanceUsesLaterBootstrapAndKeepsRetryingFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	available, err := NewNode(ctx, testConfig(t, "second-bootstrap", true))
	if err != nil {
		t.Fatal(err)
	}
	defer available.Close()
	unreachableID := writeTestKey(t, filepath.Join(t.TempDir(), "unreachable.key"), "first-bootstrap")
	unreachable := fmt.Sprintf("/ip4/127.0.0.1/udp/1/quic-v1/p2p/%s", unreachableID)

	cfg := testConfig(t, "multiple-bootstrap-normal", false)
	cfg.BootstrapAddresses = []string{unreachable, firstFullAddress(t, available)}
	cfg.Limits.TargetOutboundPeers = 2
	cfg.Limits.DialTimeout = 50 * time.Millisecond
	cfg.Limits.HandshakeTimeout = 50 * time.Millisecond
	client, err := NewNode(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	waitUsable(t, client, available.PeerID(), true)
	waitForMaintenanceAttempts(t, client, 2)
	if client.IsUsable(unreachableID) {
		t.Fatal("unreachable bootstrap became usable")
	}
}

func TestOutboundMaintenanceStopsWithNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	unreachableID := writeTestKey(t, filepath.Join(t.TempDir(), "shutdown.key"), "shutdown-bootstrap")
	cfg := testConfig(t, "shutdown-normal", false)
	cfg.BootstrapAddresses = []string{fmt.Sprintf("/ip4/127.0.0.1/udp/1/quic-v1/p2p/%s", unreachableID)}
	cfg.Limits.TargetOutboundPeers = 1
	cfg.Limits.DialTimeout = 50 * time.Millisecond
	node, err := NewNode(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitForMaintenanceAttempts(t, node, 2)
	if err := node.Close(); err != nil {
		t.Fatal(err)
	}
	afterClose := node.maintenanceAttempts.Load()
	time.Sleep(3 * cfg.Limits.BackoffMaximum)
	if attempts := node.maintenanceAttempts.Load(); attempts != afterClose {
		t.Fatalf("maintenance continued after Close: %d -> %d", afterClose, attempts)
	}
}
