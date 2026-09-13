package consensus

import (
	"bytes"
	"context"
	"crypto/ed25519"
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
	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/protocol"
)

const (
	networkStartupTimeout = 20 * time.Second
	finalityTimeout       = 30 * time.Second
	quorumLossWindow      = 1500 * time.Millisecond
)

type loopbackTCPReservation struct {
	listener net.Listener
	address  string
}

func reserveLoopbackAddress(t testing.TB) *loopbackTCPReservation {
	t.Helper()
	reservation, err := reserveLoopbackAddressAt("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reservation.release() })
	return reservation
}

func reserveLoopbackAddressAt(address string) (*loopbackTCPReservation, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return &loopbackTCPReservation{listener: listener, address: listener.Addr().String()}, nil
}

func (reservation *loopbackTCPReservation) release() error {
	if reservation == nil || reservation.listener == nil {
		return nil
	}
	err := reservation.listener.Close()
	reservation.listener = nil
	return err
}

type localValidator struct {
	index   int
	root    string
	address string
	config  *config.Config
	app     *Application
	privKey cmted25519.PrivKey
	nodeKey *p2p.NodeKey
	node    *node.Node
	service *Service
	port    *loopbackTCPReservation
}

type localNetwork struct {
	genesis Genesis
	nodes   []*localValidator
}

func newLocalNetwork(t testing.TB) *localNetwork {
	t.Helper()
	validators, validatorKeys := testValidators()
	genesis, err := NewGenesis(protocol.Alpha1NetworkID, validators)
	if err != nil {
		t.Fatal(err)
	}
	return newLocalNetworkWithGenesis(t, genesis, validatorKeys, false)
}

func newLocalNetworkWithGenesis(t testing.TB, genesis Genesis, validatorKeys []cmted25519.PrivKey, emptyBlocks bool) *localNetwork {
	t.Helper()
	network := &localNetwork{genesis: genesis, nodes: make([]*localValidator, len(validatorKeys))}
	for i := range network.nodes {
		root := filepath.Join(t.TempDir(), fmt.Sprintf("validator-%d", i))
		app, err := NewApplication(genesis)
		if err != nil {
			t.Fatal(err)
		}
		port := reserveLoopbackAddress(t)
		network.nodes[i] = &localValidator{
			index: i, root: root, address: port.address, app: app, privKey: validatorKeys[i], port: port,
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
		cfg.Consensus.CreateEmptyBlocks = emptyBlocks
		if emptyBlocks {
			cfg.Consensus.CreateEmptyBlocksInterval = 50 * time.Millisecond
		}
		cfg.Consensus.TimeoutPropose = 150 * time.Millisecond
		cfg.Consensus.TimeoutProposeDelta = 20 * time.Millisecond
		cfg.Consensus.TimeoutPrevote = 50 * time.Millisecond
		cfg.Consensus.TimeoutPrevoteDelta = 10 * time.Millisecond
		cfg.Consensus.TimeoutPrecommit = 50 * time.Millisecond
		cfg.Consensus.TimeoutPrecommitDelta = 10 * time.Millisecond
		cfg.Consensus.TimeoutCommit = 50 * time.Millisecond
		cfg.Consensus.SkipTimeoutCommit = false
		cfg.Consensus.PeerQueryMaj23SleepDuration = 10 * time.Millisecond
		cfg.TxIndex.Indexer = "null"
		validator.config = cfg
		network.initializeFiles(t, validator)
	}
	network.refreshPersistentPeers()
	t.Cleanup(func() { network.stopAll() })
	return network
}

func (network *localNetwork) refreshPersistentPeers() {
	for _, validator := range network.nodes {
		peers := make([]string, 0, len(network.nodes)-1)
		for _, peer := range network.nodes {
			if peer.index != validator.index {
				peers = append(peers, fmt.Sprintf("%s@%s", peer.nodeKey.ID(), peer.address))
			}
		}
		validator.config.P2P.PersistentPeers = strings.Join(peers, ",")
	}
}

func (network *localNetwork) reserveFreshPort(t testing.TB, validator *localValidator) {
	t.Helper()
	port := reserveLoopbackAddress(t)
	validator.port = port
	validator.address = port.address
	validator.config.P2P.ListenAddress = "tcp://" + port.address
	validator.config.P2P.ExternalAddress = "tcp://" + port.address
	network.refreshPersistentPeers()
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
	created, err := NewService(validator.config, validator.app)
	if err != nil {
		return err
	}
	validator.service = created
	return nil
}

func (network *localNetwork) start(t testing.TB, indexes ...int) {
	t.Helper()
	for _, index := range indexes {
		validator := network.nodes[index]
		if validator.port == nil {
			network.reserveFreshPort(t, validator)
		}
		if err := validator.port.release(); err != nil {
			t.Fatalf("release validator %d port reservation: %v", index, err)
		}
		validator.port = nil
		if err := network.makeNode(validator); err != nil {
			t.Fatalf("make validator %d: %v", index, err)
		}
		if err := validator.service.Start(context.Background()); err != nil {
			t.Fatalf("start validator %d: %v", index, err)
		}
		validator.node = validator.service.Node()
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
	_ = validator.service.Stop(context.Background())
	validator.node = nil
	validator.service = nil
	// Preserve the validator's advertised address across an in-test restart.
	// This prevents the still-running peers from retaining a stale persistent
	// peer address while also keeping the port unavailable to other processes.
	if port, err := reserveLoopbackAddressAt(validator.address); err == nil {
		validator.port = port
	}
}

func (network *localNetwork) stopAll() {
	for _, validator := range network.nodes {
		network.stop(validator.index)
		_ = validator.port.release()
		validator.port = nil
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

func (network *localNetwork) waitConvergedState(t testing.TB, indexes []int, predicate func(Status) bool) []Status {
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
			for i := 1; i < len(statuses); i++ {
				// Observing live nodes is not an atomic read: one validator may
				// already expose the next empty/recovery height while all nodes
				// have the same committed canonical application state.
				if !bytes.Equal(statuses[i].AppHash, statuses[0].AppHash) ||
					!bytes.Equal(mustSnapshotBytes(t, statuses[i].Snapshot), mustSnapshotBytes(t, statuses[0].Snapshot)) {
					ready = false
					break
				}
			}
		}
		if ready {
			return statuses
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for validator convergence")
	return nil
}

func requireStateConverged(t testing.TB, statuses []Status) {
	t.Helper()
	for i := 1; i < len(statuses); i++ {
		if !bytes.Equal(statuses[i].AppHash, statuses[0].AppHash) ||
			!bytes.Equal(mustSnapshotBytes(t, statuses[i].Snapshot), mustSnapshotBytes(t, statuses[0].Snapshot)) {
			t.Fatalf("validator %d did not converge on application state", i)
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
	port := reserveLoopbackAddress(t)
	validator := &localValidator{index: 3, root: filepath.Join(t.TempDir(), "wrong-network"),
		address: port.address, app: app, privKey: validatorKeys[3], port: port,
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
	if err := validator.port.release(); err != nil {
		t.Fatal(err)
	}
	validator.port = nil
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
	statuses := network.waitConvergedState(t, []int{0, 1, 2, 3}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 1
	})
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
	statuses = network.waitConvergedState(t, []int{0, 1, 2, 3}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 1 && status.Snapshot.Identities[0].Sequence == 1
	})
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
	statuses = network.waitConvergedState(t, []int{0, 1, 2}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 2
	})
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
	statuses = network.waitConvergedState(t, []int{0, 1, 2}, func(status Status) bool {
		return len(status.Snapshot.Identities) == 3 && status.Height > beforeA.Height
	})
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

type governanceIntegration struct {
	state      chain.State
	memberKeys []ed25519.PrivateKey
	members    []identity.IdentityGenesisProof
	validators []governance.Validator
}

func governanceIntegrationState(t testing.TB, consensusValidators []Validator) governanceIntegration {
	t.Helper()
	fixture := governanceIntegration{state: chain.Genesis(protocol.Alpha1NetworkID)}
	for i, start := range []byte{0, 32, 64, 96} {
		tx, proof := makeIdentityTx(t, start, protocol.ProtocolTimestamp(i+1))
		var err error
		fixture.state, _, err = chain.Apply(fixture.state, tx)
		if err != nil {
			t.Fatal(err)
		}
		fixture.memberKeys = append(fixture.memberKeys, deterministicMemberKey(start))
		fixture.members = append(fixture.members, proof)
	}
	for i, validator := range consensusValidators {
		fixture.validators = append(fixture.validators, governance.Validator{Operator: fixture.members[i%3].IdentityID,
			PublicKey: append([]byte(nil), validator.PublicKey...), Power: governance.ValidatorPower})
	}
	var err error
	fixture.state, err = chain.BootstrapGovernance(fixture.state,
		[]identity.IdentityID{fixture.members[0].IdentityID, fixture.members[1].IdentityID, fixture.members[2].IdentityID}, fixture.validators)
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func governanceProposalTx(t testing.TB, body governance.ProposalBody, key ed25519.PrivateKey) chain.Transaction {
	t.Helper()
	authorization, err := governance.CreateProposal(body, key)
	if err != nil {
		t.Fatal(err)
	}
	return chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: chain.GovernanceProposal, GovernanceProposal: &authorization}
}

func governanceVoteTx(t testing.TB, proposalID governance.ProposalID, voter identity.IdentityID, key ed25519.PrivateKey) chain.Transaction {
	t.Helper()
	authorization, err := governance.CreateVote(governance.VoteBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		ProposalID: proposalID, Voter: voter, Choice: governance.Yes}, key)
	if err != nil {
		t.Fatal(err)
	}
	return chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID,
		Type: chain.GovernanceVote, GovernanceVote: &authorization}
}

func governanceActionTx(t testing.TB, transactionType chain.TransactionType, proposalID governance.ProposalID,
	submitter identity.IdentityID, key ed25519.PrivateKey) chain.Transaction {
	t.Helper()
	body := governance.ActionBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		ProposalID: proposalID, Submitter: submitter}
	var authorization governance.ActionAuthorization
	var err error
	if transactionType == chain.GovernanceFinalize {
		authorization, err = governance.CreateFinalize(body, key)
	} else {
		authorization, err = governance.CreateExecute(body, key)
	}
	if err != nil {
		t.Fatal(err)
	}
	tx := chain.Transaction{SchemaVersion: chain.TransactionSchema, NetworkID: protocol.Alpha1NetworkID, Type: transactionType}
	if transactionType == chain.GovernanceFinalize {
		tx.GovernanceFinalize = &authorization
	} else {
		tx.GovernanceExecute = &authorization
	}
	return tx
}

func submitChainTx(t testing.TB, network *localNetwork, nodeIndex int, tx chain.Transaction) []byte {
	t.Helper()
	raw, err := tx.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	network.submit(t, nodeIndex, raw)
	return raw
}

func proposalSnapshot(snapshot chain.Snapshot, proposalID governance.ProposalID) (governance.ProposalSnapshot, bool) {
	if snapshot.Governance == nil {
		return governance.ProposalSnapshot{}, false
	}
	for _, proposal := range snapshot.Governance.Proposals {
		if proposal.ID.String() == proposalID.String() {
			return proposal, true
		}
	}
	return governance.ProposalSnapshot{}, false
}

func waitProposal(t testing.TB, network *localNetwork, proposalID governance.ProposalID, indexes []int,
	predicate func(governance.ProposalSnapshot) bool) []Status {
	t.Helper()
	return network.waitState(t, indexes, func(status Status) bool {
		proposal, ok := proposalSnapshot(status.Snapshot, proposalID)
		return ok && predicate(proposal)
	})
}

func approveProposal(t testing.TB, network *localNetwork, proposal chain.Transaction, fixture governanceIntegration, indexes []int) ([]Status, []byte) {
	t.Helper()
	proposalID := proposal.GovernanceProposal.ProposalID
	submitChainTx(t, network, 0, proposal)
	statuses := waitProposal(t, network, proposalID, indexes, func(proposal governance.ProposalSnapshot) bool { return proposal.Status == governance.Open })
	opened, _ := proposalSnapshot(statuses[0].Snapshot, proposalID)
	for i := 0; i < 3; i++ {
		submitChainTx(t, network, i, governanceVoteTx(t, proposalID, fixture.members[i].IdentityID, fixture.memberKeys[i]))
	}
	waitProposal(t, network, proposalID, indexes, func(proposal governance.ProposalSnapshot) bool { return len(proposal.Votes) == 3 })
	network.waitHeight(t, opened.EndHeight, indexes...)
	finalize := governanceActionTx(t, chain.GovernanceFinalize, proposalID, fixture.members[0].IdentityID, fixture.memberKeys[0])
	submitChainTx(t, network, 0, finalize)
	statuses = waitProposal(t, network, proposalID, indexes, func(proposal governance.ProposalSnapshot) bool { return proposal.Status == governance.Approved })
	requireStateConverged(t, statuses)
	execute := governanceActionTx(t, chain.GovernanceExecute, proposalID, fixture.members[0].IdentityID, fixture.memberKeys[0])
	raw := submitChainTx(t, network, 0, execute)
	statuses = waitProposal(t, network, proposalID, indexes, func(proposal governance.ProposalSnapshot) bool { return proposal.Status == governance.Executed })
	requireStateConverged(t, statuses)
	return statuses, raw
}

func transactionHeight(t testing.TB, network *localNetwork, raw []byte, maximum int64) int64 {
	t.Helper()
	for height := int64(1); height <= maximum; height++ {
		block, _ := network.nodes[0].node.BlockStore().LoadBlock(height)
		if block == nil {
			continue
		}
		for _, transaction := range block.Data.Txs {
			if bytes.Equal(transaction, raw) {
				return height
			}
		}
	}
	t.Fatal("committed transaction height not found")
	return 0
}

func requireEngineValidatorSet(t testing.TB, validator *localValidator, height int64, expected ...[]byte) {
	t.Helper()
	environment, err := validator.node.ConfigureRPC()
	if err != nil {
		t.Fatal(err)
	}
	result, err := environment.Validators(nil, &height, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != len(expected) {
		t.Fatalf("engine validator count=%d, want %d at height %d", result.Total, len(expected), height)
	}
	for _, publicKey := range expected {
		found := false
		for _, validator := range result.Validators {
			if bytes.Equal(validator.PubKey.Bytes(), publicKey) && validator.VotingPower == governance.ValidatorPower {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("equal-power validator %x missing at height %d", publicKey, height)
		}
	}
}

func TestGovernanceThroughCometBFTAndValidatorSetChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("governance validator-set loopback integration test")
	}
	consensusValidators, consensusKeys := testValidators()
	fixture := governanceIntegrationState(t, consensusValidators)
	genesis, err := NewGenesisWithState(protocol.Alpha1NetworkID, consensusValidators, fixture.state)
	if err != nil {
		t.Fatal(err)
	}
	validatorEKey := cmted25519.GenPrivKeyFromSecret([]byte(testKeyNotice + "E"))
	network := newLocalNetworkWithGenesis(t, genesis, append(consensusKeys, validatorEKey), true)
	network.start(t, 0, 1, 2, 3)
	active := []int{0, 1, 2, 3}

	membershipBody := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		Kind: governance.MembershipChange, Proposer: fixture.members[0].IdentityID, CreatedAt: 100,
		Membership: &governance.MembershipChangePayload{Target: fixture.members[3].IdentityID,
			Expected: membership.Pending, Requested: membership.Active}}
	membershipProposal := governanceProposalTx(t, membershipBody, fixture.memberKeys[0])
	statuses, _ := approveProposal(t, network, membershipProposal, fixture, active)
	if memberStatus(statuses[0].Snapshot, fixture.members[3].IdentityID) != membership.Active {
		t.Fatal("four-validator governance execution did not activate Dave")
	}

	setHash, err := fixture.state.Governance.ValidatorSetHash()
	if err != nil {
		t.Fatal(err)
	}
	validatorE := governance.Validator{Operator: fixture.members[3].IdentityID,
		PublicKey: append([]byte(nil), validatorEKey.PubKey().Bytes()...), Power: governance.ValidatorPower}
	addBody := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		Kind: governance.ValidatorSetChange, Proposer: fixture.members[0].IdentityID, CreatedAt: 200,
		ValidatorSet: &governance.ValidatorSetChangePayload{Action: governance.AddValidator, Validator: validatorE, ExpectedSetHash: setHash}}
	addProposal := governanceProposalTx(t, addBody, fixture.memberKeys[0])
	statuses, addRaw := approveProposal(t, network, addProposal, fixture, active)
	addHeight := transactionHeight(t, network, addRaw, statuses[0].Height)
	network.waitHeight(t, addHeight+2, active...)

	// E starts from the original four-validator genesis, block-syncs every
	// governance decision, and becomes effective under CometBFT's H+2 rule.
	network.start(t, 4)
	activeWithE := []int{0, 1, 2, 3, 4}
	statuses = network.waitHeight(t, addHeight+3, activeWithE...)
	requireStateConverged(t, statuses)
	expectedAfterAdd := make([][]byte, 0, 5)
	for _, validator := range consensusValidators {
		expectedAfterAdd = append(expectedAfterAdd, validator.PublicKey)
	}
	expectedAfterAdd = append(expectedAfterAdd, validatorE.PublicKey)
	requireEngineValidatorSet(t, network.nodes[0], addHeight+2, expectedAfterAdd...)
	active = activeWithE
	proofTx, _ := makeIdentityTx(t, 128, 300)
	submitChainTx(t, network, 4, proofTx)
	statuses = network.waitState(t, active, func(status Status) bool { return len(status.Snapshot.Identities) == 5 })
	requireStateConverged(t, statuses)

	setHashAfterAdd := validatorSetHashFromSnapshot(t, statuses[0].Snapshot)
	removeBody := governance.ProposalBody{SchemaVersion: governance.Schema, NetworkID: protocol.Alpha1NetworkID,
		Kind: governance.ValidatorSetChange, Proposer: fixture.members[1].IdentityID, CreatedAt: 201,
		ValidatorSet: &governance.ValidatorSetChangePayload{Action: governance.RemoveValidator,
			Validator: fixture.validators[3], ExpectedSetHash: setHashAfterAdd}}
	removeProposal := governanceProposalTx(t, removeBody, fixture.memberKeys[1])
	statuses, removeRaw := approveProposal(t, network, removeProposal, fixture, active)
	removeHeight := transactionHeight(t, network, removeRaw, statuses[0].Height)
	network.waitHeight(t, removeHeight+2, active...)

	expectedAfterRemove := make([][]byte, 0, 4)
	for i := 0; i < 3; i++ {
		expectedAfterRemove = append(expectedAfterRemove, consensusValidators[i].PublicKey)
	}
	expectedAfterRemove = append(expectedAfterRemove, validatorE.PublicKey)
	requireEngineValidatorSet(t, network.nodes[0], removeHeight+2, expectedAfterRemove...)
	// D remains connected as a non-validator full node. The engine-derived
	// four-validator set continues finalizing and all five applications converge.
	proofTx, _ = makeIdentityTx(t, 160, 301)
	submitChainTx(t, network, 4, proofTx)
	statuses = network.waitState(t, active, func(status Status) bool { return len(status.Snapshot.Identities) == 6 })
	requireStateConverged(t, statuses)
	if statuses[0].Snapshot.Governance == nil || len(statuses[0].Snapshot.Governance.Validators) != 4 {
		t.Fatal("canonical validator set did not converge to four equal-power validators")
	}
	for _, validator := range statuses[0].Snapshot.Governance.Validators {
		if validator.Power != governance.ValidatorPower {
			t.Fatal("validator power changed from one")
		}
	}
}

func memberStatus(snapshot chain.Snapshot, memberID identity.IdentityID) membership.Status {
	for _, member := range snapshot.Memberships {
		if member.ID.String() == memberID.String() {
			return member.Status
		}
	}
	return ""
}

func validatorSetHashFromSnapshot(t testing.TB, snapshot chain.Snapshot) protocol.HashDigest {
	t.Helper()
	if snapshot.Governance == nil {
		t.Fatal("missing governance snapshot")
	}
	state, err := governance.NewState(snapshot.Governance.Validators)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := state.ValidatorSetHash()
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
