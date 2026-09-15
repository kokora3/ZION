package node

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	cmtconfig "github.com/cometbft/cometbft/config"
	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmtp2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/kokora3/zion/internal/api"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
)

func fixtureTransaction(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "chain", "testdata", "identity_create_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]any
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(fixture["canonical_transaction_cbor_hex"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func testRuntimeConfig(t testing.TB, name string, authoritative, listen bool) Config {
	t.Helper()
	directory := t.TempDir()
	fingerprint := protocol.HashBytes([]byte("phase-8-test-genesis"))
	cfg := DefaultConfig(directory, fingerprint, chain.Genesis(protocol.Alpha1NetworkID))
	cfg.FreshStateIsAuthoritative = authoritative
	cfg.API.Listen = "127.0.0.1:0"
	cfg.P2P.KeyPath = filepath.Join(directory, name, "peer.key")
	cfg.P2P.PeerCachePath = filepath.Join(directory, name, "peers.json")
	if listen {
		cfg.P2P.ListenAddresses = []string{"/ip4/127.0.0.1/udp/0/quic-v1"}
	} else {
		cfg.P2P.ListenAddresses = nil
	}
	cfg.P2P.Limits.DialTimeout = time.Second
	cfg.P2P.Limits.HandshakeTimeout = 2 * time.Second
	cfg.P2P.Limits.PEXTimeout = 2 * time.Second
	cfg.SyncInterval = 100 * time.Millisecond
	cfg.SyncTimeout = 2 * time.Second
	return cfg
}

func TestBootstrapStatusReportsBindAdvertiseAndCopyableMultiaddr(t *testing.T) {
	cfg := testRuntimeConfig(t, "public-status", true, true)
	cfg.P2P.Roles = []p2p.Role{p2p.RoleNormal, p2p.RoleBootstrap}
	cfg.P2P.AdvertiseAddresses = []string{"/dns4/bootstrap.public.test/udp/42000/quic-v1"}
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(context.Background())

	status := runtime.Status().(map[string]any)
	peerID := status["peer_id"].(string)
	advertised := status["p2p_advertised_addresses"].([]string)
	bound := status["p2p_listen_addresses"].([]string)
	bootstrap := status["bootstrap_multiaddrs"].([]string)
	if len(advertised) != 1 || advertised[0] != cfg.P2P.AdvertiseAddresses[0] {
		t.Fatalf("advertised status = %v", advertised)
	}
	if len(bound) == 0 || !strings.Contains(bound[0], "/ip4/127.0.0.1/udp/") {
		t.Fatalf("listen status = %v", bound)
	}
	want := cfg.P2P.AdvertiseAddresses[0] + "/p2p/" + peerID
	if len(bootstrap) != 1 || bootstrap[0] != want {
		t.Fatalf("bootstrap status = %v, want %s", bootstrap, want)
	}
	if status["peer_count"].(int) != 0 || status["validator_authorized"].(bool) {
		t.Fatal("fresh bootstrap status gained peers or validator authority")
	}
}

type testConsensus struct {
	mu     sync.RWMutex
	active bool
	submit func([]byte) error
}

func (s *testConsensus) Start(context.Context) error {
	s.mu.Lock()
	s.active = true
	s.mu.Unlock()
	return nil
}
func (s *testConsensus) Stop(context.Context) error {
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
	return nil
}
func (s *testConsensus) Active() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.active }
func (s *testConsensus) Submit(_ context.Context, raw []byte) error {
	return s.submit(append([]byte(nil), raw...))
}

func TestStateStorePersistenceVerificationAndCorruption(t *testing.T) {
	cfg := testRuntimeConfig(t, "store", true, false)
	store, err := NewStateStore(cfg.StatePath, cfg.NetworkID, cfg.GenesisID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != ErrSnapshotNotFound {
		t.Fatalf("missing snapshot: %v", err)
	}
	raw := fixtureTransaction(t)
	tx, err := consensus.DecodeTransaction(raw, cfg.NetworkID)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := chain.Apply(cfg.InitialState, tx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(state, 1); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	wantHash, _ := state.Hash()
	gotHash, _ := loaded.State.Hash()
	if loaded.Height != 1 || wantHash.String() != gotHash.String() {
		t.Fatal("persisted state did not round trip")
	}

	original, err := os.ReadFile(cfg.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	var valid storedSnapshot
	if err := json.Unmarshal(original, &valid); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(cfg.StatePath, cfg.StatePath+".previous"); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Load()
	if err != nil || recovered.Height != 1 {
		t.Fatalf("interrupted replacement backup was not recovered: %v", err)
	}
	if _, err := os.Stat(cfg.StatePath); err != nil {
		t.Fatal("recovered snapshot was not promoted:", err)
	}
	invalid := state
	invalid.NetworkID = "another-network"
	if err := store.Save(invalid, 2); err == nil {
		t.Fatal("invalid snapshot save succeeded")
	}
	afterFailedSave, err := os.ReadFile(cfg.StatePath)
	if err != nil || !bytes.Equal(afterFailedSave, original) {
		t.Fatal("failed save damaged the previous usable snapshot")
	}
	mutations := []func(*storedSnapshot){
		func(v *storedSnapshot) { v.StorageVersion++ },
		func(v *storedSnapshot) { v.NetworkID = "another-network" },
		func(v *storedSnapshot) { v.GenesisHex = strings.Repeat("00", 32) },
		func(v *storedSnapshot) { v.StateHash = "zion:state:sha256:" + strings.Repeat("00", 32) },
		func(v *storedSnapshot) { v.CanonicalState[len(v.CanonicalState)-1] ^= 0xff },
	}
	for i, mutate := range mutations {
		candidate := valid
		candidate.CanonicalState = append([]byte(nil), valid.CanonicalState...)
		mutate(&candidate)
		data, _ := json.Marshal(candidate)
		if err := os.WriteFile(cfg.StatePath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatalf("corruption mutation %d accepted", i)
		}
	}
	if err := os.WriteFile(cfg.StatePath, original[:len(original)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("truncated snapshot accepted")
	}
	if err := os.WriteFile(cfg.StatePath, bytes.Repeat([]byte("x"), maxEnvelopeBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("oversized snapshot accepted")
	}
}

func TestOutboundRuntimeSyncRelayAPIBootstrapLossAndRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	serverCfg := testRuntimeConfig(t, "validator", true, true)
	service := &testConsensus{}
	serverCfg.Consensus = service
	server, err := New(serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	var height int64
	service.submit = func(raw []byte) error { height++; _, err := server.Commit(raw, height); return err }
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())

	bootstrapCfg := testRuntimeConfig(t, "bootstrap", true, true)
	bootstrapCfg.GenesisID = serverCfg.GenesisID
	bootstrapCfg.P2P.NetworkFingerprint = serverCfg.GenesisID
	bootstrapCfg.P2P.Roles = []p2p.Role{p2p.RoleBootstrap, p2p.RoleNormal}
	bootstrap, err := New(bootstrapCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Stop(context.Background())
	if err := server.P2P().Dial(ctx, bootstrap.P2P().FullAddresses()[0].String(), p2p.SourceBootstrap); err != nil {
		t.Fatal(err)
	}

	normalCfg := testRuntimeConfig(t, "normal", false, false)
	normalCfg.GenesisID = serverCfg.GenesisID
	normalCfg.P2P.NetworkFingerprint = serverCfg.GenesisID
	normalCfg.P2P.BootstrapAddresses = []string{bootstrap.P2P().FullAddresses()[0].String()}
	normalCfg.P2P.Limits.TargetOutboundPeers = 2
	normal, err := New(normalCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := normal.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if len(normal.P2P().ListenAddresses()) != 0 {
		t.Fatal("outbound-only runtime opened a P2P listener")
	}
	normalPeerID := normal.P2P().PeerID()
	defer normal.Stop(context.Background())
	waitRuntime(t, normal, func() bool { normal.mu.RLock(); defer normal.mu.RUnlock(); return normal.sync == Synced })

	raw := fixtureTransaction(t)
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "transaction": base64.StdEncoding.EncodeToString(raw)})
	response := apiRequest(t, http.MethodPost, "http://"+normal.APIAddress()+"/v1/transactions", wrapper)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("transaction API returned %d: %s", response.StatusCode, response.Body)
	}
	waitRuntime(t, server, func() bool { server.mu.RLock(); defer server.mu.RUnlock(); return server.height == 1 })
	serverHash := runtimeHash(t, server)
	waitRuntime(t, normal, func() bool { return runtimeHash(t, normal) == serverHash })
	normalHash := runtimeHash(t, normal)
	if serverHash != normalHash {
		t.Fatal("normal node did not converge after relayed finalization")
	}
	tx, _ := consensus.DecodeTransaction(raw, protocol.Alpha1NetworkID)
	txID, _ := tx.ID()
	statusResult := apiRequest(t, http.MethodGet, "http://"+normal.APIAddress()+"/v1/transactions/"+txID.String(), nil)
	if statusResult.StatusCode != http.StatusOK || !bytes.Contains(statusResult.Body, []byte("FINALIZED")) {
		t.Fatalf("normal node did not observe finalized transaction status: %s", statusResult.Body)
	}
	if err := bootstrap.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !normal.P2P().IsUsable(server.P2P().PeerID()) {
		t.Fatal("bootstrap loss broke direct validator peer")
	}

	for _, endpoint := range []string{"/v1/health", "/v1/status", "/v1/peers", "/v1/state", "/v1/governance/proposals"} {
		result := apiRequest(t, http.MethodGet, "http://"+normal.APIAddress()+endpoint, nil)
		if result.StatusCode != http.StatusOK {
			t.Fatalf("%s returned %d", endpoint, result.StatusCode)
		}
		if strings.Contains(strings.ToLower(string(result.Body)), "private") {
			t.Fatal("API response appears to expose private material")
		}
	}
	if err := normal.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	normalCfg.P2P.BootstrapAddresses = nil
	restarted, err := New(normalCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop(context.Background())
	if restarted.P2P().PeerID() != normalPeerID {
		t.Fatal("PeerID changed after restart")
	}
	waitRuntime(t, restarted, func() bool { return restarted.P2P().IsUsable(server.P2P().PeerID()) })
	if runtimeHash(t, restarted) != serverHash {
		t.Fatal("restart did not restore synchronized state")
	}
}

func TestRunningOutboundRuntimeReconnectsAndResynchronizesAfterBootstrapRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	bootstrapCfg := testRuntimeConfig(t, "persistent-runtime-bootstrap", true, true)
	bootstrapCfg.P2P.Roles = []p2p.Role{p2p.RoleNormal, p2p.RoleBootstrap}
	bootstrap, err := New(bootstrapCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Stop(context.Background())
	bootstrapID := bootstrap.P2P().PeerID()
	bootstrapAddress := bootstrap.P2P().FullAddresses()[0].String()
	bootstrapCfg.P2P.ListenAddresses = []string{strings.TrimSuffix(bootstrapAddress, "/p2p/"+bootstrapID.String())}

	normalCfg := testRuntimeConfig(t, "persistent-runtime-normal", false, false)
	normalCfg.P2P.BootstrapAddresses = []string{bootstrapAddress}
	normalCfg.P2P.Limits.TargetOutboundPeers = 1
	normalCfg.P2P.Limits.DialTimeout = 100 * time.Millisecond
	normalCfg.P2P.Limits.HandshakeTimeout = 100 * time.Millisecond
	normalCfg.P2P.Limits.BackoffInitial = 20 * time.Millisecond
	normalCfg.P2P.Limits.BackoffMaximum = 100 * time.Millisecond
	normalCfg.SyncTimeout = 500 * time.Millisecond
	normal, err := New(normalCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := normal.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer normal.Stop(context.Background())
	waitRuntime(t, normal, func() bool {
		connected, outbound := normal.P2P().ConnectionCounts()
		normal.mu.RLock()
		defer normal.mu.RUnlock()
		return connected == 1 && outbound == 1 && normal.sync == Synced
	})

	if err := bootstrap.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitRuntime(t, normal, func() bool {
		connected, outbound := normal.P2P().ConnectionCounts()
		return connected == 0 && outbound == 0
	})

	restarted, err := New(bootstrapCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop(context.Background())
	if restarted.P2P().PeerID() != bootstrapID {
		t.Fatal("persistent bootstrap PeerID changed across runtime recreation")
	}
	waitRuntime(t, normal, func() bool {
		connected, outbound := normal.P2P().ConnectionCounts()
		normal.mu.RLock()
		defer normal.mu.RUnlock()
		return connected == 1 && outbound == 1 && normal.sync == Synced
	})
}

const runtimePortBindAttempts = 5

type runtimeTCPReservation struct {
	listener net.Listener
	address  string
}

func reserveRuntimeTCPAddress(t testing.TB) *runtimeTCPReservation {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	reservation := &runtimeTCPReservation{listener: listener, address: listener.Addr().String()}
	t.Cleanup(func() { _ = reservation.release() })
	return reservation
}

func (reservation *runtimeTCPReservation) release() error {
	if reservation == nil || reservation.listener == nil {
		return nil
	}
	err := reservation.listener.Close()
	reservation.listener = nil
	return err
}

func isRuntimeAddressInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "address already in use") ||
		strings.Contains(message, "only one usage of each socket address")
}

type validatorRuntimeHarness struct {
	runtime *Runtime
	config  Config
	comet   *cmtconfig.Config
	app     *consensus.Application
}

func newValidatorRuntimeHarness(t testing.TB, genesis consensus.Genesis, privateKeys []cmted25519.PrivKey,
	validators []consensus.Validator,
) ([]validatorRuntimeHarness, []*runtimeTCPReservation) {
	t.Helper()
	nodes := make([]validatorRuntimeHarness, len(privateKeys))
	ports := make([]*runtimeTCPReservation, len(nodes))
	addresses := make([]string, len(nodes))
	for index := range addresses {
		ports[index] = reserveRuntimeTCPAddress(t)
		addresses[index] = ports[index].address
	}
	for index := range nodes {
		root := filepath.Join(t.TempDir(), "validator")
		comet := cmtconfig.TestConfig().SetRoot(root)
		comet.Moniker = validators[index].Name
		comet.DBBackend = "pebbledb"
		comet.RPC.ListenAddress = ""
		comet.GRPC.ListenAddress = ""
		comet.P2P.ListenAddress = "tcp://" + addresses[index]
		comet.P2P.ExternalAddress = "tcp://" + addresses[index]
		comet.P2P.AddrBookStrict = false
		comet.P2P.AllowDuplicateIP = true
		comet.P2P.PexReactor = false
		comet.P2P.PersistentPeersMaxDialPeriod = 100 * time.Millisecond
		comet.Consensus.CreateEmptyBlocks = false
		comet.Consensus.TimeoutPropose = 150 * time.Millisecond
		comet.Consensus.TimeoutPrevote = 50 * time.Millisecond
		comet.Consensus.TimeoutPrecommit = 50 * time.Millisecond
		comet.Consensus.TimeoutCommit = 50 * time.Millisecond
		comet.TxIndex.Indexer = "null"
		nodeKey := &cmtp2p.NodeKey{PrivKey: cmted25519.GenPrivKeyFromSecret([]byte("TEST ONLY PUBLIC PHASE 8 COMET P2P " + string(rune('A'+index))))}
		peers := make([]string, 0, len(nodes)-1)
		for peerIndex := range nodes {
			if peerIndex == index {
				continue
			}
			peerKey := cmted25519.GenPrivKeyFromSecret([]byte("TEST ONLY PUBLIC PHASE 8 COMET P2P " + string(rune('A'+peerIndex))))
			peers = append(peers, string(cmtp2p.PubKeyToID(peerKey.PubKey()))+"@"+addresses[peerIndex])
		}
		comet.P2P.PersistentPeers = strings.Join(peers, ",")
		for _, directory := range []string{filepath.Dir(comet.GenesisFile()), filepath.Dir(comet.PrivValidatorStateFile())} {
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := genesis.CometGenesis(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)).SaveAs(comet.GenesisFile()); err != nil {
			t.Fatal(err)
		}
		pv := privval.NewFilePV(privateKeys[index], comet.PrivValidatorKeyFile(), comet.PrivValidatorStateFile())
		pv.Save()
		if err := nodeKey.SaveAs(comet.NodeKeyFile()); err != nil {
			t.Fatal(err)
		}
		cfg := DefaultConfig(root, genesis.GenesisID, genesis.InitialState)
		cfg.FreshStateIsAuthoritative = true
		cfg.API.Listen = "127.0.0.1:0"
		cfg.P2P.ListenAddresses = []string{"/ip4/127.0.0.1/udp/0/quic-v1"}
		cfg.P2P.Roles = []p2p.Role{p2p.RoleNormal, p2p.RoleValidator}
		cfg.ValidatorPublicKey = append([]byte(nil), privateKeys[index].PubKey().Bytes()...)
		for _, validator := range validators {
			cfg.GenesisValidatorKeys = append(cfg.GenesisValidatorKeys, append([]byte(nil), validator.PublicKey...))
		}
		entry := &nodes[index]
		entry.config = cfg
		entry.comet = comet
		entry.config.ConsensusFactory = func(state chain.State, height int64) (ConsensusService, error) {
			app, err := consensus.NewApplicationFromState(genesis, state, height)
			if err != nil {
				return nil, err
			}
			entry.app = app
			return consensus.NewService(comet, app)
		}
		created, err := New(entry.config)
		if err != nil {
			t.Fatal(err)
		}
		entry.runtime = created
	}
	return nodes, ports
}

func startValidatorRuntimeHarness(ctx context.Context, nodes []validatorRuntimeHarness,
	ports []*runtimeTCPReservation,
) (int, error) {
	for index := range nodes {
		if err := ports[index].release(); err != nil {
			return index, fmt.Errorf("release validator runtime %d port reservation: %w", index, err)
		}
		if err := nodes[index].runtime.Start(ctx); err != nil {
			for pending := index + 1; pending < len(ports); pending++ {
				_ = ports[pending].release()
			}
			return index, fmt.Errorf("start validator runtime %d: %w", index, err)
		}
	}
	return len(nodes), nil
}

func stopValidatorRuntimeHarness(nodes []validatorRuntimeHarness, count int) {
	quiesceValidatorRuntimeHarness(nodes, count)
	for index := 0; index < count; index++ {
		_ = nodes[index].runtime.Stop(context.Background())
	}
}

func quiesceValidatorRuntimeHarness(nodes []validatorRuntimeHarness, count int) {
	stoppedSwitch := false
	for index := 0; index < count; index++ {
		nodes[index].runtime.mu.RLock()
		service, ok := nodes[index].runtime.consensus.(*consensus.Service)
		nodes[index].runtime.mu.RUnlock()
		if !ok || service.Node() == nil || !service.Node().Switch().IsRunning() {
			continue
		}
		_ = service.Node().Switch().Stop()
		service.Node().Switch().Wait()
		stoppedSwitch = true
	}
	if stoppedSwitch {
		// CometBFT v1.0.1 does not join per-peer consensus gossip routines when
		// its switch stops. Give those routines several configured 10 ms query
		// intervals to observe the stopped peer/reactor before Runtime closes the
		// underlying Pebble stores. This is test-harness lifecycle coordination.
		time.Sleep(250 * time.Millisecond)
	}
}

func TestFourValidatorRuntimeFinalityPersistenceAndRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	privateKeys := make([]cmted25519.PrivKey, consensus.DefaultValidatorCount)
	validators := make([]consensus.Validator, consensus.DefaultValidatorCount)
	for index := range privateKeys {
		privateKeys[index] = cmted25519.GenPrivKeyFromSecret([]byte("TEST ONLY PUBLIC PHASE 8 VALIDATOR " + string(rune('A'+index))))
		validators[index] = consensus.Validator{Name: "phase8-validator-" + string(rune('a'+index)),
			PublicKey: privateKeys[index].PubKey().Bytes(), Power: consensus.DefaultValidatorPower}
	}
	genesis, err := consensus.NewGenesis(protocol.Alpha1NetworkID, validators)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []validatorRuntimeHarness
	for attempt := 1; attempt <= runtimePortBindAttempts; attempt++ {
		candidate, ports := newValidatorRuntimeHarness(t, genesis, privateKeys, validators)
		started, startErr := startValidatorRuntimeHarness(ctx, candidate, ports)
		if startErr == nil {
			nodes = candidate
			break
		}
		stopValidatorRuntimeHarness(candidate, started)
		if !isRuntimeAddressInUse(startErr) || attempt == runtimePortBindAttempts {
			t.Fatal(startErr)
		}
	}
	for index := range nodes {
		defer nodes[index].runtime.Stop(context.Background())
		if len(nodes[index].runtime.P2P().ListenAddresses()) == 0 {
			t.Fatal("validator runtime did not start general P2P")
		}
	}
	raw := fixtureTransaction(t)
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "transaction": base64.StdEncoding.EncodeToString(raw)})
	result := apiRequest(t, http.MethodPost, "http://"+nodes[0].runtime.APIAddress()+"/v1/transactions", wrapper)
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("validator API submission failed: %d %s", result.StatusCode, result.Body)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		converged := true
		for index := range nodes {
			nodes[index].runtime.mu.RLock()
			ready := nodes[index].runtime.height >= 1
			nodes[index].runtime.mu.RUnlock()
			if !ready {
				converged = false
				break
			}
		}
		if converged {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	wantHash := runtimeHash(t, nodes[0].runtime)
	for index := range nodes {
		nodes[index].runtime.mu.RLock()
		height := nodes[index].runtime.height
		nodes[index].runtime.mu.RUnlock()
		if height < 1 || runtimeHash(t, nodes[index].runtime) != wantHash {
			t.Fatalf("validator runtime %d did not converge", index)
		}
		status, err := nodes[index].app.Status()
		if err != nil || "zion:state:sha256:"+hex.EncodeToString(status.AppHash) != wantHash {
			t.Fatalf("validator runtime %d AppHash mismatch", index)
		}
	}
	quiesceValidatorRuntimeHarness(nodes, len(nodes))
	for index := range nodes {
		if err := nodes[index].runtime.Stop(context.Background()); err != nil {
			t.Fatalf("stop validator runtime %d: %v", index, err)
		}
	}
	// This restart verifies only durable application state, so it does not need
	// a known consensus peer address. Let the real CometBFT listener bind port 0
	// directly, avoiding a second allocate-close-bind handoff.
	nodes[0].comet.P2P.ListenAddress = "tcp://127.0.0.1:0"
	nodes[0].comet.P2P.ExternalAddress = ""
	nodes[0].comet.P2P.PersistentPeers = ""
	restarted, err := New(nodes[0].config)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Start(ctx); err != nil {
		t.Fatal("restart validator runtime:", err)
	}
	defer restarted.Stop(context.Background())
	if runtimeHash(t, restarted) != wantHash {
		t.Fatal("validator restart did not restore persisted StateHash")
	}
}

type httpResult struct {
	StatusCode int
	Body       []byte
}

func apiRequest(t testing.TB, method, url string, body []byte) httpResult {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return httpResult{response.StatusCode, data}
}

func waitRuntime(t testing.TB, runtime *Runtime, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runtime condition timeout")
}

func runtimeHash(t testing.TB, runtime *Runtime) string {
	t.Helper()
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	hash, err := runtime.state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return hash.String()
}

func TestStateSyncRejectsRollbackConflictAndFailedInstallIsAtomic(t *testing.T) {
	cfg := testRuntimeConfig(t, "sync-atomic", true, false)
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	runtime.state = cloneState(cfg.InitialState)
	runtime.height = 2
	runtime.sync = Synced
	before := runtimeHash(t, runtime)
	offer, err := runtime.currentOfferForState(cfg.InitialState, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.installOffer(offer, cfg.InitialState); err == nil {
		t.Fatal("rollback accepted")
	}
	if runtimeHash(t, runtime) != before {
		t.Fatal("rollback rejection mutated state")
	}

	raw := fixtureTransaction(t)
	tx, _ := consensus.DecodeTransaction(raw, cfg.NetworkID)
	conflictState, _, _ := chain.Apply(cfg.InitialState, tx)
	conflict, _ := runtime.currentOfferForState(conflictState, 2)
	if err := runtime.installOffer(conflict, conflictState); err == nil {
		t.Fatal("equal-height conflict accepted")
	}
	if runtimeHash(t, runtime) != before {
		t.Fatal("conflict rejection mutated state")
	}
}

func TestStateOfferVerificationMatrix(t *testing.T) {
	cfg := testRuntimeConfig(t, "offer-matrix", true, false)
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := runtime.currentOfferForState(cfg.InitialState, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.verifyOffer(valid); err != nil {
		t.Fatal("valid state offer rejected:", err)
	}
	mutations := []func(*StateSnapshotOffer){
		func(offer *StateSnapshotOffer) { offer.NetworkID = "another-network" },
		func(offer *StateSnapshotOffer) {
			offer.NetworkFingerprint = protocol.HashBytes([]byte("wrong-genesis"))
		},
		func(offer *StateSnapshotOffer) {
			offer.StateHash = chain.StateHash{HashDigest: protocol.HashBytes([]byte("wrong-state"))}
		},
		func(offer *StateSnapshotOffer) {
			offer.CanonicalState = append(append([]byte(nil), offer.CanonicalState...), 0)
		},
	}
	for index, mutate := range mutations {
		candidate := valid
		candidate.CanonicalState = append([]byte(nil), valid.CanonicalState...)
		mutate(&candidate)
		if _, err := runtime.verifyOffer(candidate); err == nil {
			t.Fatalf("invalid state offer mutation %d accepted", index)
		}
	}
}

func (r *Runtime) currentOfferForState(state chain.State, height int64) (StateSnapshotOffer, error) {
	canonical, err := state.CanonicalBytes()
	if err != nil {
		return StateSnapshotOffer{}, err
	}
	hash, err := state.Hash()
	if err != nil {
		return StateSnapshotOffer{}, err
	}
	return StateSnapshotOffer{SchemaVersion: RuntimeWireSchema, NetworkID: r.cfg.NetworkID,
		NetworkFingerprint: r.cfg.GenesisID, Height: height, StateHash: hash, CanonicalState: canonical}, nil
}

func TestAPIRejectsMalformedOversizedAndUnsupportedRequests(t *testing.T) {
	cfg := testRuntimeConfig(t, "api-errors", true, false)
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(context.Background())
	base := "http://" + runtime.APIAddress()
	cases := []struct {
		method, path string
		body         []byte
		want         int
	}{
		{http.MethodPost, "/v1/transactions", []byte("{"), http.StatusBadRequest},
		{http.MethodPost, "/v1/transactions", bytes.Repeat([]byte("x"), int(cfg.API.MaxBodyBytes)+1), http.StatusRequestEntityTooLarge},
		{http.MethodGet, "/v1/transactions/not-a-txid", nil, http.StatusBadRequest},
		{http.MethodGet, "/v1/governance/proposals?limit=9999", nil, http.StatusBadRequest},
		{http.MethodDelete, "/v1/state", nil, http.StatusMethodNotAllowed},
		{http.MethodGet, "/unknown", nil, http.StatusNotFound},
	}
	for _, test := range cases {
		result := apiRequest(t, test.method, base+test.path, test.body)
		if result.StatusCode != test.want {
			t.Fatalf("%s %s: got %d want %d body=%s", test.method, test.path, result.StatusCode, test.want, result.Body)
		}
		if bytes.Contains(result.Body, []byte("goroutine")) {
			t.Fatal("API leaked stack trace")
		}
	}
}

func TestAPIContractAndFinalitySemantics(t *testing.T) {
	cfg := testRuntimeConfig(t, "api-contract", true, false)
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(context.Background())
	raw := fixtureTransaction(t)
	receipt, err := runtime.Commit(raw, 1)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := consensus.DecodeTransaction(raw, cfg.NetworkID)
	if err != nil {
		t.Fatal(err)
	}
	identityID := tx.IdentityCreate.IdentityID.String()
	proposalID := governance.ProposalID{HashDigest: protocol.HashBytes([]byte("unknown-proposal"))}.String()
	unknownIdentity := tx.IdentityCreate.IdentityID
	unknownIdentity.Digest = protocol.HashBytes([]byte("unknown-identity")).Digest
	base := "http://" + runtime.APIAddress()
	cases := []struct {
		path string
		want int
	}{
		{"/v1/health", http.StatusOK}, {"/v1/status", http.StatusOK}, {"/v1/peers", http.StatusOK},
		{"/v1/state", http.StatusOK}, {"/v1/identities/" + identityID, http.StatusOK},
		{"/v1/memberships/" + identityID, http.StatusOK}, {"/v1/governance/proposals", http.StatusOK},
		{"/v1/governance/proposals/" + proposalID, http.StatusNotFound},
		{"/v1/identities/" + unknownIdentity.String(), http.StatusNotFound},
		{"/v1/transactions/" + receipt.TxID.String(), http.StatusOK},
	}
	for _, test := range cases {
		result := apiRequest(t, http.MethodGet, base+test.path, nil)
		if result.StatusCode != test.want {
			t.Fatalf("GET %s returned %d, want %d: %s", test.path, result.StatusCode, test.want, result.Body)
		}
		lower := bytes.ToLower(result.Body)
		if bytes.Contains(lower, []byte("private_key")) || bytes.Contains(lower, []byte("seed")) {
			t.Fatalf("GET %s exposed secret-shaped data", test.path)
		}
	}
	status := apiRequest(t, http.MethodGet, base+"/v1/transactions/"+receipt.TxID.String(), nil)
	if !bytes.Contains(status.Body, []byte("FINALIZED")) {
		t.Fatal("committed transaction was not reported FINALIZED")
	}
	for _, forbidden := range []string{"/v1/admin/activate-member", "/v1/admin/add-validator"} {
		if result := apiRequest(t, http.MethodPost, base+forbidden, []byte(`{}`)); result.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("governance bypass route %s unexpectedly exists", forbidden)
		}
	}
}

func TestGovernanceCanonicalBytesPassThroughTransactionAPI(t *testing.T) {
	fixtureData, err := os.ReadFile(filepath.Join("..", "governance", "testdata", "governance_proposal_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Canonical string `json:"canonical_cbor_hex"`
	}
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(fixture.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testRuntimeConfig(t, "governance-api", false, false)
	service := &testConsensus{}
	received := make(chan []byte, 1)
	service.submit = func(candidate []byte) error {
		received <- append([]byte(nil), candidate...)
		return nil
	}
	cfg.Consensus = service
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(context.Background())
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "transaction": base64.StdEncoding.EncodeToString(raw)})
	result := apiRequest(t, http.MethodPost, "http://"+runtime.APIAddress()+"/v1/transactions", wrapper)
	if result.StatusCode != http.StatusAccepted || !bytes.Contains(result.Body, []byte("SUBMITTED_TO_CONSENSUS")) {
		t.Fatalf("governance API submission failed: %d %s", result.StatusCode, result.Body)
	}
	select {
	case forwarded := <-received:
		if !bytes.Equal(forwarded, raw) {
			t.Fatal("API altered canonical governance transaction bytes")
		}
	case <-time.After(time.Second):
		t.Fatal("governance transaction did not reach consensus submission")
	}
}

func TestTransactionRelayRejectsWrongNetworkOversizeAndDuplicates(t *testing.T) {
	cfg := testRuntimeConfig(t, "relay-validation", true, false)
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Stop(context.Background())
	if _, err := runtime.SubmitTransaction(context.Background(), bytes.Repeat([]byte{0}, chain.MaxTransactionBytes+1)); err == nil {
		t.Fatal("oversized relay transaction accepted")
	}
	raw := fixtureTransaction(t)
	tx, err := consensus.DecodeTransaction(raw, cfg.NetworkID)
	if err != nil {
		t.Fatal(err)
	}
	transactionID, _ := tx.ID()
	tx.NetworkID = "another-network"
	wrongNetwork, err := tx.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SubmitTransaction(context.Background(), wrongNetwork); err == nil {
		t.Fatal("wrong-network relay transaction accepted")
	}
	runtime.mu.Lock()
	runtime.remember(TransactionRecord{TxID: transactionID.String(), Status: "SUBMITTED"})
	runtime.mu.Unlock()
	known, err := runtime.SubmitTransaction(context.Background(), raw)
	if err != nil || known.Status != "KNOWN_ALREADY" {
		t.Fatal("bounded TxID duplicate was not recognized before re-relay")
	}
}

func TestLifecycleAndPartialStartupFailuresCleanUp(t *testing.T) {
	cfg := testRuntimeConfig(t, "lifecycle", true, false)
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("duplicate Start accepted")
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal("idempotent Stop failed:", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	blockedCfg := testRuntimeConfig(t, "api-bind-failure", true, false)
	blockedCfg.API.Listen = listener.Addr().String()
	blocked, err := New(blockedCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := blocked.Start(context.Background()); err == nil {
		t.Fatal("occupied API port accepted")
	}
	blocked.mu.RLock()
	failedState := blocked.lifecycle
	blocked.mu.RUnlock()
	if failedState != Failed {
		t.Fatalf("partial startup state=%s", failedState)
	}
	_ = listener.Close()
	retry, err := New(blockedCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.Start(context.Background()); err != nil {
		t.Fatal("restart after cleanup failed:", err)
	}
	if err := retry.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	badP2P := testRuntimeConfig(t, "p2p-bind-failure", true, true)
	badP2P.P2P.ListenAddresses = []string{"/not-a-multiaddr"}
	badRuntime, err := New(badP2P)
	if err != nil {
		t.Fatal(err)
	}
	if err := badRuntime.Start(context.Background()); err == nil {
		t.Fatal("invalid P2P listener accepted")
	}

	corruptCfg := testRuntimeConfig(t, "corrupt-start", true, false)
	if err := os.MkdirAll(filepath.Dir(corruptCfg.StatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corruptCfg.StatePath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	corruptRuntime, err := New(corruptCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := corruptRuntime.Start(context.Background()); err == nil {
		t.Fatal("corrupt state silently reset")
	}
	data, _ := os.ReadFile(corruptCfg.StatePath)
	if string(data) != "corrupt" {
		t.Fatal("failed startup destructively changed corrupt snapshot")
	}
}

func TestConsensusInitializationFailureDoesNotPersistFreshState(t *testing.T) {
	cfg := testRuntimeConfig(t, "consensus-init-failure", true, false)
	cfg.ConsensusFactory = func(chain.State, int64) (ConsensusService, error) {
		return nil, io.ErrUnexpectedEOF
	}
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("consensus initialization failure was accepted")
	}
	if _, err := os.Stat(cfg.StatePath); !os.IsNotExist(err) {
		t.Fatal("invalid fresh validator state was persisted before consensus validation")
	}
}

func TestAPIRemoteBindingAndCORSAreFailClosed(t *testing.T) {
	cfg := testRuntimeConfig(t, "api-security", true, false)
	runtime, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	remote := cfg.API
	remote.Listen = "0.0.0.0:0"
	if _, err := api.NewServer(remote, runtime); err == nil {
		t.Fatal("unauthenticated remote API binding accepted")
	}
	remote.BearerToken = "a-long-test-only-token"
	server, err := api.NewServer(remote, runtime)
	if err != nil {
		t.Fatal("authenticated remote API config rejected:", err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Close(context.Background())
	_, port, err := net.SplitHostPort(server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+"/v1/health", nil)
	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("remote API accepted an unauthenticated request")
	}
	local := cfg.API
	local.AllowedOrigins = []string{"*"}
	if _, err := api.NewServer(local, runtime); err == nil {
		t.Fatal("wildcard CORS accepted")
	}
	cors := cfg.API
	cors.AllowedOrigins = []string{"http://127.0.0.1:3000"}
	corsServer, err := api.NewServer(cors, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := corsServer.Start(); err != nil {
		t.Fatal(err)
	}
	defer corsServer.Close(context.Background())
	preflight, _ := http.NewRequest(http.MethodOptions, "http://"+corsServer.Addr()+"/v1/transactions", nil)
	preflight.Header.Set("Origin", "http://127.0.0.1:3000")
	preflightResponse, err := (&http.Client{Timeout: time.Second}).Do(preflight)
	if err != nil {
		t.Fatal(err)
	}
	_ = preflightResponse.Body.Close()
	if preflightResponse.StatusCode != http.StatusNoContent || preflightResponse.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("explicit CORS preflight was not allowed")
	}
}

func TestWireDecoderBoundsAndCanonicality(t *testing.T) {
	raw := fixtureTransaction(t)
	message := TxRelayMessage{SchemaVersion: RuntimeWireSchema, Transaction: raw}
	encoded, err := EncodeTxRelay(message, protocol.Alpha1NetworkID)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeTxRelay(encoded, protocol.Alpha1NetworkID); err != nil || !bytes.Equal(decoded.Transaction, raw) {
		t.Fatal("transaction relay round trip failed")
	}
	if _, err := DecodeTxRelay(append(encoded, 0), protocol.Alpha1NetworkID); err == nil {
		t.Fatal("non-canonical relay accepted")
	}
	if _, err := DecodeStateOffer(bytes.Repeat([]byte{0}, MaxStateFrame+1)); err == nil {
		t.Fatal("oversized state offer accepted")
	}
}

func TestRuntimeWireGoldenFixtures(t *testing.T) {
	load := func(name string) map[string]any {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		var fixture map[string]any
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatal(err)
		}
		return fixture
	}
	txFixture := load("tx_submit_golden.json")
	raw, err := hex.DecodeString(txFixture["transaction_cbor_hex"].(string))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeTxRelay(TxRelayMessage{SchemaVersion: RuntimeWireSchema, Transaction: raw}, protocol.Alpha1NetworkID)
	if err != nil || hex.EncodeToString(encoded) != txFixture["canonical_cbor_hex"].(string) ||
		txFixture["protocol_id"].(string) != string(TxProtocolID) {
		t.Fatal("transaction relay golden mismatch")
	}
	decoded, err := DecodeTxRelay(encoded, protocol.Alpha1NetworkID)
	if err != nil || !bytes.Equal(decoded.Transaction, raw) {
		t.Fatal("transaction relay golden decode failed")
	}
	logicalTx, err := consensus.DecodeTransaction(decoded.Transaction, protocol.Alpha1NetworkID)
	logicalID, idErr := logicalTx.ID()
	if err != nil || idErr != nil || string(logicalTx.NetworkID) != txFixture["network_id"].(string) ||
		string(logicalTx.Type) != txFixture["transaction_type"].(string) || logicalID.String() != txFixture["tx_id"].(string) {
		t.Fatal("transaction relay logical golden fields mismatch")
	}

	stateFixture := load("state_snapshot_offer_golden.json")
	fingerprint, err := hex.DecodeString(stateFixture["network_fingerprint_hex"].(string))
	if err != nil {
		t.Fatal(err)
	}
	state := chain.Genesis(protocol.Alpha1NetworkID)
	stateBytes, _ := state.CanonicalBytes()
	stateHash, _ := state.Hash()
	offer := StateSnapshotOffer{SchemaVersion: RuntimeWireSchema, NetworkID: protocol.Alpha1NetworkID,
		NetworkFingerprint: protocol.HashDigest{Algorithm: protocol.HashAlgorithmSHA256, Digest: fingerprint},
		Height:             0, StateHash: stateHash, CanonicalState: stateBytes}
	offerBytes, err := EncodeStateOffer(offer)
	if err != nil || hex.EncodeToString(offerBytes) != stateFixture["canonical_cbor_hex"].(string) ||
		stateFixture["protocol_id"].(string) != string(StateProtocolID) {
		t.Fatal("state snapshot offer golden mismatch")
	}
	decodedOffer, err := DecodeStateOffer(offerBytes)
	if err != nil || decodedOffer.StateHash.String() != stateFixture["state_hash"].(string) ||
		hex.EncodeToString(decodedOffer.CanonicalState) != stateFixture["canonical_state_cbor_hex"].(string) ||
		string(decodedOffer.NetworkID) != stateFixture["network_id"].(string) || decodedOffer.Height != int64(stateFixture["height"].(float64)) {
		t.Fatal("state snapshot offer golden decode failed")
	}
}

func FuzzDecodeTxRelayMessage(f *testing.F) {
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeTxRelay(data, protocol.Alpha1NetworkID) })
}

func FuzzDecodeStateSyncMessage(f *testing.F) {
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeStateOffer(data) })
}
