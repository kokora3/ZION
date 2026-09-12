package consensus

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cometbft/cometbft/config"
	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

const (
	networkStartupTimeout = 20 * time.Second
	finalityTimeout       = 15 * time.Second
	quorumLossWindow      = 1500 * time.Millisecond
)

type localValidator struct {
	index   int
	root    string
	address string
	config  *config.Config
	app     *Application
	privKey cmted25519.PrivKey
	nodeKey *p2p.NodeKey
	node    *node.Node
}

type localNetwork struct {
	genesis Genesis
	nodes   []*localValidator
}

func freeLoopbackAddress(t testing.TB) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func newLocalNetwork(t testing.TB) *localNetwork {
	t.Helper()
	validators, validatorKeys := testValidators()
	genesis, err := NewGenesis(protocol.Alpha1NetworkID, validators)
	if err != nil {
		t.Fatal(err)
	}
	network := &localNetwork{genesis: genesis, nodes: make([]*localValidator, DefaultValidatorCount)}
	for i := range network.nodes {
		root := filepath.Join(t.TempDir(), fmt.Sprintf("validator-%d", i))
		app, err := NewApplication(genesis)
		if err != nil {
			t.Fatal(err)
		}
		network.nodes[i] = &localValidator{
			index: i, root: root, address: freeLoopbackAddress(t), app: app, privKey: validatorKeys[i],
			nodeKey: &p2p.NodeKey{PrivKey: cmted25519.GenPrivKeyFromSecret([]byte(fmt.Sprintf("%s-p2p-%d", testKeyNotice, i)))},
		}
	}

	for _, validator := range network.nodes {
		cfg := config.TestConfig().SetRoot(validator.root)
		cfg.Moniker = fmt.Sprintf("zion-test-validator-%d", validator.index)
		cfg.DBBackend = "pebbledb"
		cfg.RPC.ListenAddress = ""
		cfg.GRPC.ListenAddress = ""
		cfg.P2P.ListenAddress = "tcp://" + validator.address
		cfg.P2P.ExternalAddress = "tcp://" + validator.address
		cfg.P2P.AddrBookStrict = false
		cfg.P2P.AllowDuplicateIP = true
		cfg.P2P.PexReactor = false
		cfg.P2P.PersistentPeersMaxDialPeriod = 100 * time.Millisecond
		cfg.Consensus.CreateEmptyBlocks = false
		cfg.Consensus.TimeoutPropose = 150 * time.Millisecond
		cfg.Consensus.TimeoutProposeDelta = 20 * time.Millisecond
		cfg.Consensus.TimeoutPrevote = 50 * time.Millisecond
		cfg.Consensus.TimeoutPrevoteDelta = 10 * time.Millisecond
		cfg.Consensus.TimeoutPrecommit = 50 * time.Millisecond
		cfg.Consensus.TimeoutPrecommitDelta = 10 * time.Millisecond
		cfg.Consensus.TimeoutCommit = 50 * time.Millisecond
		cfg.Consensus.SkipTimeoutCommit = false
		cfg.TxIndex.Indexer = "null"
		peers := make([]string, 0, len(network.nodes)-1)
		for _, peer := range network.nodes {
			if peer.index != validator.index {
				peers = append(peers, fmt.Sprintf("%s@%s", peer.nodeKey.ID(), peer.address))
			}
		}
		cfg.P2P.PersistentPeers = strings.Join(peers, ",")
		validator.config = cfg
		network.initializeFiles(t, validator)
	}
	t.Cleanup(func() { network.stopAll() })
	return network
}

func (network *localNetwork) initializeFiles(t testing.TB, validator *localValidator) {
	t.Helper()
	for _, directory := range []string{filepath.Dir(validator.config.GenesisFile()), filepath.Dir(validator.config.PrivValidatorStateFile())} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	genesisTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := network.genesis.CometGenesis(genesisTime).SaveAs(validator.config.GenesisFile()); err != nil {
		t.Fatal(err)
	}
	filePV := privval.NewFilePV(validator.privKey, validator.config.PrivValidatorKeyFile(), validator.config.PrivValidatorStateFile())
	filePV.Save()
	if err := validator.nodeKey.SaveAs(validator.config.NodeKeyFile()); err != nil {
		t.Fatal(err)
	}
}

func (network *localNetwork) makeNode(validator *localValidator) error {
	filePV := privval.LoadFilePV(validator.config.PrivValidatorKeyFile(), validator.config.PrivValidatorStateFile())
	nodeKey, err := p2p.LoadNodeKey(validator.config.NodeKeyFile())
	if err != nil {
		return err
	}
	created, err := node.NewNode(context.Background(), validator.config, filePV, nodeKey,
		proxy.NewLocalClientCreator(validator.app), node.DefaultGenesisDocProviderFunc(validator.config),
		config.DefaultDBProvider, node.DefaultMetricsProvider(validator.config.Instrumentation), cmtlog.NewNopLogger())
	if err != nil {
		return err
	}
	validator.node = created
	return nil
}

func (network *localNetwork) start(t testing.TB, indexes ...int) {
	t.Helper()
	for _, index := range indexes {
		validator := network.nodes[index]
		if err := network.makeNode(validator); err != nil {
			t.Fatalf("make validator %d: %v", index, err)
		}
		if err := validator.node.Start(); err != nil {
			t.Fatalf("start validator %d: %v", index, err)
		}
	}
	deadline := time.Now().Add(networkStartupTimeout)
	for time.Now().Before(deadline) {
		ready := true
		for _, index := range indexes {
			if network.nodes[index].node == nil || !network.nodes[index].node.IsRunning() {
				ready = false
			}
		}
		if ready {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("validator network startup timeout")
}

func (network *localNetwork) stop(index int) {
	validator := network.nodes[index]
	if validator.node == nil {
		return
	}
	_ = validator.node.Stop()
	validator.node.Wait()
	validator.node = nil
}

func (network *localNetwork) stopAll() {
	for i := range network.nodes {
		network.stop(i)
	}
}

func (network *localNetwork) submit(t testing.TB, index int, raw []byte) {
	t.Helper()
	request, err := network.nodes[index].node.Mempool().CheckTx(cmttypes.Tx(raw), "")
	if err != nil {
		t.Fatalf("submit transaction: %v", err)
	}
	request.Wait()
	response := request.Response.GetCheckTx()
	if response == nil || response.Code != CodeOK {
		t.Fatalf("CheckTx rejected transaction: %+v", response)
	}
}

func (network *localNetwork) waitHeight(t testing.TB, height int64, indexes ...int) []Status {
	return network.waitState(t, indexes, func(status Status) bool { return status.Height >= height })
}

func (network *localNetwork) waitState(t testing.TB, indexes []int, predicate func(Status) bool) []Status {
	t.Helper()
	deadline := time.Now().Add(finalityTimeout)
	for time.Now().Before(deadline) {
		statuses := make([]Status, len(indexes))
		ready := true
		for i, index := range indexes {
			status, err := network.nodes[index].app.Status()
			if err != nil || !predicate(status) {
				ready = false
				break
			}
			statuses[i] = status
		}
		if ready {
			return statuses
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for committed application state")
	return nil
}

func requireConverged(t testing.TB, statuses []Status) {
	t.Helper()
	for i := 1; i < len(statuses); i++ {
		if statuses[i].Height != statuses[0].Height || !bytes.Equal(statuses[i].AppHash, statuses[0].AppHash) ||
			!bytes.Equal(mustSnapshotBytes(t, statuses[i].Snapshot), mustSnapshotBytes(t, statuses[0].Snapshot)) {
			t.Fatalf("validator %d did not converge", i)
		}
	}
}

func mustSnapshotBytes(t testing.TB, snapshot chain.Snapshot) []byte {
	t.Helper()
	raw, err := protocol.CanonicalEncode(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func requireExpectedAppHash(t testing.TB, status Status, state chain.State) {
	t.Helper()
	hash, err := state.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(status.AppHash, hash.Digest) {
		t.Fatalf("CometBFT AppHash %x != Phase 4 StateHash digest %x", status.AppHash, hash.Digest)
	}
}

func startIncompatibleNode(t testing.TB, network *localNetwork) *localValidator {
	t.Helper()
	validators, validatorKeys := testValidators()
	wrongGenesis, err := NewGenesis("wrong-network", validators)
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewApplication(wrongGenesis)
	if err != nil {
		t.Fatal(err)
	}
	validator := &localValidator{index: 3, root: filepath.Join(t.TempDir(), "wrong-network"),
		address: freeLoopbackAddress(t), app: app, privKey: validatorKeys[3],
		nodeKey: &p2p.NodeKey{PrivKey: cmted25519.GenPrivKeyFromSecret([]byte(testKeyNotice + "-wrong-network-p2p"))}}
	cfg := config.TestConfig().SetRoot(validator.root)
	cfg.Moniker = "zion-incompatible-validator"
	cfg.DBBackend = "pebbledb"
	cfg.RPC.ListenAddress = ""
	cfg.GRPC.ListenAddress = ""
	cfg.P2P.ListenAddress = "tcp://" + validator.address
	cfg.P2P.ExternalAddress = "tcp://" + validator.address
	cfg.P2P.AddrBookStrict = false
	cfg.P2P.AllowDuplicateIP = true
	cfg.P2P.PexReactor = false
	cfg.P2P.PersistentPeersMaxDialPeriod = 100 * time.Millisecond
	cfg.P2P.PersistentPeers = fmt.Sprintf("%s@%s", network.nodes[0].nodeKey.ID(), network.nodes[0].address)
	cfg.Consensus.CreateEmptyBlocks = false
	cfg.TxIndex.Indexer = "null"
	validator.config = cfg
	for _, directory := range []string{filepath.Dir(cfg.GenesisFile()), filepath.Dir(cfg.PrivValidatorStateFile())} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	genesisTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := wrongGenesis.CometGenesis(genesisTime).SaveAs(cfg.GenesisFile()); err != nil {
		t.Fatal(err)
	}
	filePV := privval.NewFilePV(validator.privKey, cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile())
	filePV.Save()
	if err := validator.nodeKey.SaveAs(cfg.NodeKeyFile()); err != nil {
		t.Fatal(err)
	}
	filePV = privval.LoadFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile())
	created, err := node.NewNode(context.Background(), cfg, filePV, validator.nodeKey,
		proxy.NewLocalClientCreator(app), node.DefaultGenesisDocProviderFunc(cfg), config.DefaultDBProvider,
		node.DefaultMetricsProvider(cfg.Instrumentation), cmtlog.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	validator.node = created
	if err := created.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if validator.node != nil {
			_ = validator.node.Stop()
			validator.node.Wait()
		}
	})
	return validator
}

func TestFourValidatorCometBFTFinalityAndFaults(t *testing.T) {
	if testing.Short() {
		t.Skip("four-validator loopback integration test")
	}
	network := newLocalNetwork(t)
	network.start(t, 0, 1, 2, 3)

	create := fixtureBytes(t, "identity_create_golden.json")
	rotation := fixtureBytes(t, "key_rotation_golden.json")
	expected := chain.Genesis(protocol.Alpha1NetworkID)
	createTx, err := DecodeTransaction(create, protocol.Alpha1NetworkID)
	if err != nil {
		t.Fatal(err)
	}
	expected, _, err = chain.Apply(expected, createTx)
	if err != nil {
		t.Fatal(err)
	}
	network.submit(t, 0, create)
	statuses := network.waitState(t, []int{0, 1, 2, 3}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 1
	})
	requireConverged(t, statuses)
	requireExpectedAppHash(t, statuses[0], expected)
	if len(statuses[0].Snapshot.Identities) != 1 || statuses[0].Snapshot.Memberships[0].Status != membership.Pending {
		t.Fatalf("IdentityCreate did not finalize correctly: %+v", statuses[0].Snapshot)
	}

	rotationTx, err := DecodeTransaction(rotation, protocol.Alpha1NetworkID)
	if err != nil {
		t.Fatal(err)
	}
	expected, _, err = chain.Apply(expected, rotationTx)
	if err != nil {
		t.Fatal(err)
	}
	network.submit(t, 1, rotation)
	statuses = network.waitState(t, []int{0, 1, 2, 3}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 1 && status.Snapshot.Identities[0].Sequence == 1
	})
	requireConverged(t, statuses)
	requireExpectedAppHash(t, statuses[0], expected)
	if statuses[0].Snapshot.Identities[0].Sequence != 1 {
		t.Fatal("ordered rotation did not finalize after identity creation")
	}

	// Three equal-power validators retain the >2/3 quorum.
	network.stop(3)
	bobTx, _ := makeIdentityTx(t, 96, 100)
	bobRaw, err := bobTx.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	network.submit(t, 0, bobRaw)
	statuses = network.waitState(t, []int{0, 1, 2}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 2
	})
	requireConverged(t, statuses)
	expected, _, err = chain.Apply(expected, bobTx)
	if err != nil {
		t.Fatal(err)
	}
	requireExpectedAppHash(t, statuses[0], expected)

	// Two of four cannot finalize. The transaction remains pending, and no
	// application commits during the bounded loss-of-liveness window.
	network.stop(2)
	charlieTx, _ := makeIdentityTx(t, 128, 101)
	charlieRaw, err := charlieTx.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	network.submit(t, 0, charlieRaw)
	beforeA, _ := network.nodes[0].app.Status()
	beforeB, _ := network.nodes[1].app.Status()
	time.Sleep(quorumLossWindow)
	afterA, _ := network.nodes[0].app.Status()
	afterB, _ := network.nodes[1].app.Status()
	if afterA.Height != beforeA.Height || afterB.Height != beforeB.Height ||
		!bytes.Equal(afterA.AppHash, beforeA.AppHash) || !bytes.Equal(afterB.AppHash, beforeB.AppHash) {
		t.Fatal("two-of-four validators finalized without quorum")
	}

	// Consensus-engine storage is on disk only for this restart boundary; the
	// canonical application stays in the retained in-memory Application.
	network.start(t, 2)
	statuses = network.waitState(t, []int{0, 1, 2}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 3 && status.Height > beforeA.Height
	})
	requireConverged(t, statuses)
	expected, _, err = chain.Apply(expected, charlieTx)
	if err != nil {
		t.Fatal(err)
	}
	requireExpectedAppHash(t, statuses[0], expected)
	if len(statuses[0].Snapshot.Identities) != 3 {
		t.Fatalf("quorum recovery did not finalize pending transaction: %+v", statuses[0].Snapshot)
	}

	// A node carrying a different network/application genesis has a different
	// CometBFT chain ID, so the engine handshake never admits it as a peer.
	wrong := startIncompatibleNode(t, network)
	time.Sleep(750 * time.Millisecond)
	if wrong.node.Switch().Peers().Size() != 0 {
		t.Fatal("wrong-network validator silently joined the consensus network")
	}
}
