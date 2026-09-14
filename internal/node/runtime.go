package node

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kokora3/zion/internal/api"
	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/buildinfo"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/consensus"
	"github.com/kokora3/zion/internal/governance"
	"github.com/kokora3/zion/internal/identity"
	"github.com/kokora3/zion/internal/index"
	"github.com/kokora3/zion/internal/membership"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
)

type Lifecycle string

const (
	Created  Lifecycle = "CREATED"
	Starting Lifecycle = "STARTING"
	Running  Lifecycle = "RUNNING"
	Stopping Lifecycle = "STOPPING"
	Stopped  Lifecycle = "STOPPED"
	Failed   Lifecycle = "FAILED"
)

type SyncStatus string

const (
	Uninitialized SyncStatus = "UNINITIALIZED"
	Syncing       SyncStatus = "SYNCING"
	Synced        SyncStatus = "SYNCED"
	Stale         SyncStatus = "STALE"
	SyncError     SyncStatus = "ERROR"
)

type ConsensusService interface {
	Start(context.Context) error
	Stop(context.Context) error
	Submit(context.Context, []byte) error
	Active() bool
}

// ConsensusFactory constructs the CometBFT service only after the durable
// application snapshot has been loaded and verified.
type ConsensusFactory func(chain.State, int64) (ConsensusService, error)

type Config struct {
	NetworkID                 protocol.NetworkID
	GenesisID                 protocol.HashDigest
	StatePath                 string
	InitialState              chain.State
	InitialHeight             int64
	P2P                       p2p.Config
	API                       api.Config
	Consensus                 ConsensusService
	ConsensusFactory          ConsensusFactory
	ValidatorPublicKey        []byte
	GenesisValidatorKeys      [][]byte
	FreshStateIsAuthoritative bool
	ShutdownTimeout           time.Duration
	SyncInterval              time.Duration
	SyncTimeout               time.Duration
	RecentTransactions        int
	MaxRelayHandlers          int
	MaxSyncHandlers           int
	ObjectDirectory           string
	ObjectQuotaBytes          uint64
	ObjectFetch               objects.FetchConfig
	Board                     board.Config
	BoardIndexPath            string
	RegistryIndexPath         string
	Logger                    *slog.Logger
}

func DefaultConfig(dataDir string, genesisID protocol.HashDigest, initial chain.State) Config {
	return Config{NetworkID: initial.NetworkID, GenesisID: genesisID,
		StatePath: filepath.Join(dataDir, "state", "application.snapshot"), InitialState: initial,
		P2P: p2p.DefaultConfig(dataDir, genesisID), API: api.DefaultConfig(), ShutdownTimeout: 10 * time.Second,
		SyncInterval: 5 * time.Second, SyncTimeout: 10 * time.Second,
		RecentTransactions: 1024, MaxRelayHandlers: 16, MaxSyncHandlers: 4,
		ObjectDirectory: filepath.Join(dataDir, "objects"), ObjectQuotaBytes: objects.DefaultQuotaBytes,
		ObjectFetch: objects.DefaultFetchConfig(), Board: board.DefaultConfig(),
		BoardIndexPath:    filepath.Join(dataDir, "board", "index.json"),
		RegistryIndexPath: filepath.Join(dataDir, "registries", "index.json")}
}

type TransactionRecord struct {
	TxID            string     `json:"tx_id"`
	Status          string     `json:"status"`
	CommittedHeight int64      `json:"committed_height,omitempty"`
	ResultCode      chain.Code `json:"result_code,omitempty"`
	StateHash       string     `json:"state_hash,omitempty"`
}

type Runtime struct {
	mu            sync.RWMutex
	cfg           Config
	lifecycle     Lifecycle
	sync          SyncStatus
	state         chain.State
	height        int64
	persisted     bool
	store         *StateStore
	consensus     ConsensusService
	p2p           *p2p.Node
	objects       *objects.Store
	fetcher       *objects.Service
	board         *board.Service
	boardIndex    *index.BoardIndex
	registryIndex *index.RegistryIndex
	api           *api.Server
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	syncMu        sync.Mutex
	relaySem      chan struct{}
	syncSem       chan struct{}
	tx            map[string]TransactionRecord
	txOrder       []string
	startedAt     time.Time
	fetchSuccess  atomic.Uint64
	fetchFailure  atomic.Uint64
}

func New(cfg Config) (*Runtime, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	initial, err := cloneStateChecked(cfg.InitialState)
	if err != nil {
		return nil, fmt.Errorf("invalid initial state: %w", err)
	}
	cfg.InitialState = initial
	store, err := NewStateStore(cfg.StatePath, cfg.NetworkID, cfg.GenesisID)
	if err != nil {
		return nil, err
	}
	objectStore, err := objects.Open(cfg.ObjectDirectory, cfg.ObjectQuotaBytes)
	if err != nil {
		return nil, fmt.Errorf("open object store: %w", err)
	}
	boardIndex, err := index.OpenBoard(cfg.BoardIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open board index: %w", err)
	}
	registryIndex, err := index.OpenRegistries(cfg.RegistryIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open registry index: %w", err)
	}
	return &Runtime{cfg: cfg, lifecycle: Created, sync: Uninitialized, store: store, objects: objectStore, boardIndex: boardIndex, registryIndex: registryIndex,
		relaySem: make(chan struct{}, cfg.MaxRelayHandlers), syncSem: make(chan struct{}, cfg.MaxSyncHandlers),
		tx: make(map[string]TransactionRecord)}, nil
}

// ValidateConfig checks runtime configuration without creating files or opening listeners.
func ValidateConfig(cfg Config) error {
	if cfg.NetworkID == "" || cfg.GenesisID.Validate() != nil || cfg.StatePath == "" || cfg.InitialHeight < 0 ||
		cfg.InitialState.NetworkID != cfg.NetworkID || cfg.ShutdownTimeout <= 0 || cfg.RecentTransactions < 1 ||
		cfg.SyncInterval < 100*time.Millisecond || cfg.SyncInterval > time.Hour || cfg.SyncTimeout <= 0 || cfg.SyncTimeout > time.Minute ||
		cfg.RecentTransactions > 10000 || cfg.MaxRelayHandlers < 1 || cfg.MaxRelayHandlers > 128 ||
		cfg.MaxSyncHandlers < 1 || cfg.MaxSyncHandlers > 16 || cfg.ObjectDirectory == "" || cfg.BoardIndexPath == "" || cfg.RegistryIndexPath == "" {
		return fmt.Errorf("invalid node runtime configuration")
	}
	if cfg.P2P.NetworkID != cfg.NetworkID || !equalHash(cfg.P2P.NetworkFingerprint, cfg.GenesisID) {
		return fmt.Errorf("runtime and P2P network identity differ")
	}
	if cfg.Consensus != nil && cfg.ConsensusFactory != nil {
		return fmt.Errorf("configure either a consensus service or a consensus factory")
	}
	_, err := cloneStateChecked(cfg.InitialState)
	if err != nil {
		return fmt.Errorf("invalid initial state: %w", err)
	}
	return nil
}

func (r *Runtime) Start(parent context.Context) (err error) {
	persistInitial := false
	r.mu.Lock()
	if r.lifecycle != Created {
		state := r.lifecycle
		r.mu.Unlock()
		return fmt.Errorf("runtime cannot start from %s", state)
	}
	r.lifecycle = Starting
	r.mu.Unlock()
	defer func() {
		if err != nil {
			r.cfg.Logger.Error("node start failed", "error", err)
			_ = r.cleanup(context.Background())
			r.mu.Lock()
			r.lifecycle = Failed
			r.sync = SyncError
			r.mu.Unlock()
		}
	}()
	loaded, loadErr := r.store.Load()
	if loadErr == nil {
		r.mu.Lock()
		r.state, r.height, r.sync, r.persisted = loaded.State, loaded.Height, Synced, true
		r.mu.Unlock()
	} else if loadErr == ErrSnapshotNotFound {
		initial, cloneErr := cloneStateChecked(r.cfg.InitialState)
		if cloneErr != nil {
			return cloneErr
		}
		r.mu.Lock()
		r.state, r.height = initial, r.cfg.InitialHeight
		if r.cfg.FreshStateIsAuthoritative {
			r.sync = Synced
			persistInitial = true
		} else {
			r.sync = Uninitialized
		}
		r.mu.Unlock()
	} else {
		return loadErr
	}
	service := r.cfg.Consensus
	if r.cfg.ConsensusFactory != nil {
		r.mu.RLock()
		committed, height := r.state, r.height
		r.mu.RUnlock()
		service, err = r.cfg.ConsensusFactory(committed, height)
		if err != nil {
			return fmt.Errorf("initialize consensus: %w", err)
		}
	}
	if persistInitial {
		r.mu.RLock()
		initial, height := r.state, r.height
		r.mu.RUnlock()
		if err := r.store.Save(initial, height); err != nil {
			return err
		}
		r.mu.Lock()
		r.persisted = true
		r.mu.Unlock()
	}
	r.mu.RLock()
	indexState := cloneState(r.state)
	r.mu.RUnlock()
	if err := r.registryIndex.Rebuild(indexState); err != nil {
		return fmt.Errorf("rebuild registry index: %w", err)
	}
	r.mu.Lock()
	r.ctx, r.cancel = context.WithCancel(parent)
	r.startedAt = time.Now()
	runtimeContext := r.ctx
	r.mu.Unlock()
	p2pNode, err := p2p.NewNode(runtimeContext, r.cfg.P2P)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.p2p = p2pNode
	r.mu.Unlock()
	objectService, err := objects.NewService(runtimeContext, p2pNode, r.objects, r.cfg.NetworkID, r.cfg.ObjectFetch)
	if err != nil {
		return fmt.Errorf("start object service: %w", err)
	}
	r.mu.Lock()
	r.fetcher = objectService
	r.mu.Unlock()
	if err := r.rebuildBoardIndex(runtimeContext); err != nil {
		return fmt.Errorf("rebuild board index: %w", err)
	}
	if r.cfg.Board.Enabled {
		boardService, err := board.NewService(runtimeContext, p2pNode, r.cfg.NetworkID, r.cfg.Board, r)
		if err != nil {
			return fmt.Errorf("start board service: %w", err)
		}
		r.mu.Lock()
		r.board = boardService
		r.mu.Unlock()
	}
	r.installStreamHandlers()
	if service != nil {
		if setter, ok := service.(interface {
			SetCommitObserver(func(chain.State, int64, [][]byte) error)
		}); ok {
			setter.SetCommitObserver(r.acceptConsensusCommit)
		}
		r.mu.Lock()
		r.consensus = service
		r.mu.Unlock()
		if err := service.Start(r.ctx); err != nil {
			return fmt.Errorf("start consensus: %w", err)
		}
	}
	apiServer, err := api.NewServer(r.cfg.API, r)
	if err != nil {
		return err
	}
	if err := apiServer.Start(); err != nil {
		return fmt.Errorf("start API: %w", err)
	}
	r.mu.Lock()
	r.api = apiServer
	r.lifecycle = Running
	r.mu.Unlock()
	r.cfg.Logger.Info("node runtime started", "network", r.cfg.NetworkID, "peer_id", p2pNode.PeerID().String(),
		"roles", r.cfg.P2P.Roles, "api_listen", apiServer.Addr(), "software_version", buildinfo.Version)
	r.wg.Add(1)
	go func() { defer r.wg.Done(); r.discoverAndSync() }()
	return nil
}

func (r *Runtime) acceptConsensusCommit(state chain.State, height int64, txs [][]byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if height <= r.height {
		return fmt.Errorf("non-increasing consensus commit height")
	}
	if err := r.store.Save(state, height); err != nil {
		return err
	}
	hash, err := state.Hash()
	if err != nil {
		return err
	}
	r.state, r.height, r.sync, r.persisted = cloneState(state), height, Synced, true
	r.cfg.Logger.Info("canonical state committed", "height", height, "state_hash", hash.String(), "transactions", len(txs))
	_ = r.registryIndex.Rebuild(state)
	for _, raw := range txs {
		tx, err := consensus.DecodeTransaction(raw, r.cfg.NetworkID)
		if err != nil {
			continue
		}
		id, err := tx.ID()
		if err != nil {
			continue
		}
		r.remember(TransactionRecord{TxID: id.String(), Status: "FINALIZED", CommittedHeight: height,
			ResultCode: chain.OK, StateHash: hash.String()})
	}
	return nil
}

func (r *Runtime) Stop(ctx context.Context) error {
	r.mu.Lock()
	if r.lifecycle == Stopped {
		r.mu.Unlock()
		return nil
	}
	if r.lifecycle != Running && r.lifecycle != Failed && r.lifecycle != Starting {
		state := r.lifecycle
		r.mu.Unlock()
		return fmt.Errorf("runtime cannot stop from %s", state)
	}
	r.lifecycle = Stopping
	r.mu.Unlock()
	err := r.cleanup(ctx)
	r.mu.Lock()
	r.lifecycle = Stopped
	r.mu.Unlock()
	r.cfg.Logger.Info("node runtime stopped", "error", err)
	return err
}

func (r *Runtime) cleanup(ctx context.Context) error {
	r.mu.Lock()
	cancel, apiServer, p2pNode, objectService, boardService := r.cancel, r.api, r.p2p, r.fetcher, r.board
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
	var first error
	if apiServer != nil {
		if err := apiServer.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	if boardService != nil {
		boardService.Close()
	}
	if objectService != nil {
		objectService.Close()
	}
	r.mu.RLock()
	consensusService := r.consensus
	r.mu.RUnlock()
	if consensusService != nil {
		if err := consensusService.Stop(ctx); err != nil && first == nil {
			first = err
		}
	}
	if p2pNode != nil {
		if err := p2pNode.Close(); err != nil && first == nil {
			first = err
		}
	}
	r.mu.Lock()
	if r.api == apiServer {
		r.api = nil
	}
	if r.p2p == p2pNode {
		r.p2p = nil
	}
	if r.fetcher == objectService {
		r.fetcher = nil
	}
	if r.board == boardService {
		r.board = nil
	}
	if r.consensus == consensusService {
		r.consensus = nil
	}
	r.mu.Unlock()
	return first
}

func (r *Runtime) APIAddress() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.api == nil {
		return ""
	}
	return r.api.Addr()
}
func (r *Runtime) P2P() *p2p.Node { r.mu.RLock(); defer r.mu.RUnlock(); return r.p2p }

// ObjectStore exposes the internal Phase 9A store to later local API wiring.
func (r *Runtime) ObjectStore() *objects.Store { return r.objects }

// PutObject stores a validated canonical object in local non-consensus state.
func (r *Runtime) PutObject(ctx context.Context, object objects.Object) (protocol.ObjectID, objects.PutResult, error) {
	return r.objects.Put(ctx, object)
}

// GetObject reads and verifies one local object. It never performs networking.
func (r *Runtime) GetObject(ctx context.Context, id protocol.ObjectID) (objects.Object, error) {
	return r.objects.Get(ctx, id)
}

// StatObject reports verified local metadata without exposing filesystem paths.
func (r *Runtime) StatObject(ctx context.Context, id protocol.ObjectID) (objects.Metadata, error) {
	return r.objects.Stat(ctx, id)
}

// FetchObject performs bounded direct retrieval over the existing P2P host.
func (r *Runtime) FetchObject(ctx context.Context, id protocol.ObjectID) (objects.Object, error) {
	r.mu.RLock()
	fetcher := r.fetcher
	r.mu.RUnlock()
	if fetcher == nil {
		return objects.Object{}, fmt.Errorf("object service is not running")
	}
	object, err := fetcher.FetchObject(ctx, id)
	if err != nil {
		r.fetchFailure.Add(1)
		r.cfg.Logger.Warn("object fetch failed", "error", err)
		return objects.Object{}, err
	}
	r.fetchSuccess.Add(1)
	return object, nil
}

func (r *Runtime) Commit(raw []byte, height int64) (chain.Receipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if height != r.height+1 {
		return chain.Receipt{}, fmt.Errorf("commit height %d after %d", height, r.height)
	}
	tx, err := consensus.DecodeTransaction(raw, r.cfg.NetworkID)
	if err != nil {
		return chain.Receipt{}, err
	}
	next, receipt, err := chain.ApplyWithContext(r.state, tx, chain.ExecutionContext{Height: height})
	if err != nil {
		return receipt, err
	}
	if err := r.store.Save(next, height); err != nil {
		return chain.Receipt{}, err
	}
	r.state, r.height, r.sync, r.persisted = next, height, Synced, true
	r.remember(TransactionRecord{TxID: receipt.TxID.String(), Status: "FINALIZED", CommittedHeight: height,
		ResultCode: receipt.Code, StateHash: receipt.StateHash.String()})
	return receipt, nil
}

func (r *Runtime) remember(record TransactionRecord) {
	if _, exists := r.tx[record.TxID]; !exists {
		r.txOrder = append(r.txOrder, record.TxID)
	}
	r.tx[record.TxID] = record
	for len(r.txOrder) > r.cfg.RecentTransactions {
		delete(r.tx, r.txOrder[0])
		r.txOrder = r.txOrder[1:]
	}
}

func (r *Runtime) rememberPending(record TransactionRecord) {
	if current, exists := r.tx[record.TxID]; exists && current.Status == "FINALIZED" {
		return
	}
	r.remember(record)
}

func cloneStateChecked(state chain.State) (chain.State, error) {
	snapshot, err := state.Snapshot()
	if err != nil {
		return chain.State{}, err
	}
	return chain.StateFromSnapshot(snapshot)
}

func cloneState(state chain.State) chain.State {
	copied, err := cloneStateChecked(state)
	if err != nil {
		panic("invalid runtime state: " + err.Error())
	}
	return copied
}
func equalHash(a, b protocol.HashDigest) bool {
	return a.Algorithm == b.Algorithm && bytes.Equal(a.Digest, b.Digest)
}

func (r *Runtime) validatorAuthorized(state chain.State) bool {
	if len(r.cfg.ValidatorPublicKey) == 0 {
		return false
	}
	key := hex.EncodeToString(r.cfg.ValidatorPublicKey)
	if state.Governance != nil {
		_, ok := state.Governance.Validators[key]
		return ok
	}
	for _, candidate := range r.cfg.GenesisValidatorKeys {
		if bytes.Equal(candidate, r.cfg.ValidatorPublicKey) {
			return true
		}
	}
	return false
}

func (r *Runtime) Health() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return map[string]any{"live": r.lifecycle == Running || r.lifecycle == Starting, "ready": r.lifecycle == Running && r.sync == Synced}
}

func (r *Runtime) Status() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	hash, _ := r.state.Hash()
	peerID := ""
	peerCount, outboundPeers := 0, 0
	listenAddresses := []string{}
	advertisedAddresses := []string{}
	bootstrapMultiaddrs := []string{}
	if r.p2p != nil {
		peerID = r.p2p.PeerID().String()
		peerCount, outboundPeers = r.p2p.ConnectionCounts()
		for _, address := range r.p2p.ListenAddresses() {
			listenAddresses = append(listenAddresses, address.String())
		}
		for _, address := range r.p2p.AdvertisedAddresses() {
			advertisedAddresses = append(advertisedAddresses, address.String())
		}
		if hasRole(r.cfg.P2P.Roles, p2p.RoleBootstrap) {
			for _, address := range r.p2p.FullAddresses() {
				bootstrapMultiaddrs = append(bootstrapMultiaddrs, address.String())
			}
		}
	}
	consensusActive := r.consensus != nil && r.consensus.Active()
	objectCount, objectBytes, objectQuota := r.objects.Usage()
	posts, replies, hidden := r.boardIndex.Counts()
	boardSync := "DISABLED"
	if r.cfg.Board.Enabled && r.board != nil {
		boardSync = "ACTIVE"
	}
	return map[string]any{"network_id": r.cfg.NetworkID, "genesis_id": hex.EncodeToString(r.cfg.GenesisID.Digest),
		"runtime_state": r.lifecycle, "roles": append([]p2p.Role(nil), r.cfg.P2P.Roles...), "peer_id": peerID,
		"peer_count": peerCount, "outbound_peer_count": outboundPeers,
		"p2p_listen_addresses": listenAddresses, "p2p_advertised_addresses": advertisedAddresses,
		"bootstrap_multiaddrs": bootstrapMultiaddrs,
		"sync_status":          r.sync, "accepted_height": r.height, "state_hash": hash.String(),
		"consensus_active": consensusActive, "validator_authorized": r.validatorAuthorized(r.state),
		"protocol_version": protocol.CurrentProtocolVersion, "software_version": buildinfo.Version,
		"build_commit": buildinfo.Commit, "build_date": buildinfo.BuildDate, "object_store_enabled": true,
		"object_count": objectCount, "object_bytes": objectBytes, "object_quota_bytes": objectQuota,
		"board_enabled": r.cfg.Board.Enabled, "indexed_posts": posts, "indexed_replies": replies,
		"hidden_local": hidden, "board_sync_state": boardSync,
		"limits": map[string]any{"max_connected_peers": r.cfg.P2P.Limits.MaxConnectedPeers,
			"max_concurrent_dials": r.cfg.P2P.Limits.MaxConcurrentDials, "recent_transactions": r.cfg.RecentTransactions,
			"max_sync_snapshot_bytes": MaxSnapshotBytes, "object_max_bytes": objects.MaxObjectBytes,
			"object_quota_bytes": r.cfg.ObjectQuotaBytes}}
}

func hasRole(roles []p2p.Role, target p2p.Role) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

// Metrics returns a bounded-cardinality, non-canonical operational view.
func (r *Runtime) Metrics() api.MetricsSnapshot {
	r.mu.RLock()
	state := cloneState(r.state)
	height, lifecycle, syncStatus, startedAt := r.height, r.lifecycle, r.sync, r.startedAt
	consensusActive := r.consensus != nil && r.consensus.Active()
	validatorAuthorized := r.validatorAuthorized(r.state)
	p2pNode := r.p2p
	recent := len(r.tx)
	r.mu.RUnlock()
	connected, outbound := 0, 0
	if p2pNode != nil {
		connected, outbound = p2pNode.ConnectionCounts()
	}
	proposals := 0
	if state.Governance != nil {
		proposals = len(state.Governance.Proposals)
	}
	posts, replies, _ := r.boardIndex.Counts()
	objectCount, objectBytes, quota := r.objects.Usage()
	uptime := 0.0
	if !startedAt.IsZero() {
		uptime = time.Since(startedAt).Seconds()
	}
	return api.MetricsSnapshot{Network: string(r.cfg.NetworkID), ProtocolVersion: string(protocol.CurrentProtocolVersion),
		SoftwareVersion: buildinfo.Version, Ready: lifecycle == Running && syncStatus == Synced, SyncStatus: string(syncStatus),
		UptimeSeconds: uptime, ConnectedPeers: connected, OutboundPeers: outbound,
		MaxConnectedPeers: r.cfg.P2P.Limits.MaxConnectedPeers, ConsensusHeight: height, ConsensusActive: consensusActive,
		ValidatorAuthorized: validatorAuthorized, StateHeight: height, Identities: len(state.Identities), GovernanceProposals: proposals,
		ResearchEntries: len(state.Research), ResourceEntries: len(state.Resources), BoardPosts: posts, BoardReplies: replies,
		ObjectCount: objectCount, ObjectBytes: objectBytes, ObjectQuotaBytes: quota, RecentTransactions: recent,
		RecentTransactionLimit: r.cfg.RecentTransactions, SyncSnapshotLimitBytes: MaxSnapshotBytes,
		ObjectFetchSuccessTotal: r.fetchSuccess.Load(), ObjectFetchFailureTotal: r.fetchFailure.Load()}
}

func (r *Runtime) Peers() any {
	r.mu.RLock()
	node := r.p2p
	r.mu.RUnlock()
	if node == nil {
		return []p2p.RemotePeer{}
	}
	return node.UsablePeers()
}

func (r *Runtime) StateSummary() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	hash, _ := r.state.Hash()
	counts := map[membership.Status]int{}
	for _, value := range r.state.Memberships {
		counts[value]++
	}
	proposals, validators := 0, 0
	if r.state.Governance != nil {
		proposals = len(r.state.Governance.Proposals)
		validators = len(r.state.Governance.Validators)
	}
	return map[string]any{"schema_version": r.state.SchemaVersion, "network_id": r.state.NetworkID, "state_hash": hash.String(),
		"accepted_height": r.height, "identity_count": len(r.state.Identities), "membership_counts": counts,
		"governance_proposal_count": proposals, "validator_count": validators,
		"research_count": len(r.state.Research), "resource_count": len(r.state.Resources)}
}

func (r *Runtime) Identity(value string) (any, bool, error) {
	id, err := identity.ParseIdentityID(value)
	if err != nil {
		return nil, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	stored, ok := r.state.Identities[id.String()]
	if !ok {
		return nil, false, nil
	}
	activeKeyID := ""
	for keyID, active := range stored.Active {
		if active && (activeKeyID == "" || keyID < activeKeyID) {
			activeKeyID = keyID
		}
	}
	return map[string]any{"identity_id": stored.ID.String(), "sequence": stored.Sequence, "revoked": stored.Revoked,
		"key_count": len(stored.Keys), "active_key_id": activeKeyID, "membership": r.state.Memberships[id.String()]}, true, nil
}

func (r *Runtime) Membership(value string) (any, bool, error) {
	id, err := identity.ParseIdentityID(value)
	if err != nil {
		return nil, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	status, ok := r.state.Memberships[id.String()]
	return map[string]any{"identity_id": id.String(), "status": status}, ok, nil
}

func (r *Runtime) Proposals(offset, limit int) (any, error) {
	r.mu.RLock()
	if r.state.Governance == nil {
		r.mu.RUnlock()
		return []any{}, nil
	}
	snapshot, err := r.state.Governance.Snapshot()
	r.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if offset > len(snapshot.Proposals) {
		offset = len(snapshot.Proposals)
	}
	end := min(len(snapshot.Proposals), offset+limit)
	result := make([]GovernanceProposalView, end-offset)
	for position, proposal := range snapshot.Proposals[offset:end] {
		result[position] = governanceProposalView(proposal)
	}
	return result, nil
}

func (r *Runtime) Proposal(value string) (any, bool, error) {
	id, err := governance.ParseProposalID(value)
	if err != nil {
		return nil, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.state.Governance == nil {
		return nil, false, nil
	}
	record, ok := r.state.Governance.Proposals[id.String()]
	if !ok {
		return nil, false, nil
	}
	copyState := governance.State{Policy: r.state.Governance.Policy,
		Proposals:  map[string]governance.ProposalRecord{id.String(): record},
		Validators: map[string]governance.Validator{}}
	snapshot, err := copyState.Snapshot()
	if err != nil {
		return nil, false, err
	}
	return governanceProposalView(snapshot.Proposals[0]), true, nil
}

func (r *Runtime) Transaction(value string) (any, bool, error) {
	id, err := chain.ParseTxID(value)
	if err != nil {
		return nil, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.tx[id.String()]
	return record, ok, nil
}

var _ api.Backend = (*Runtime)(nil)
