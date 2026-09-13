package objects

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/sync/singleflight"
)

const (
	DefaultMaxObjectFetchPeers = 4
	HardMaxObjectFetchPeers    = 16
)

var (
	ErrInvalidRemoteResponse = errors.New("invalid remote object response")
	ErrObjectFetchTimeout    = errors.New("object fetch timeout")
	ErrNoAvailablePeers      = errors.New("no available object peers")
)

type FetchConfig struct {
	Timeout              time.Duration
	PeerTimeout          time.Duration
	MaxPeers             int
	MaxConcurrentFetches int
	MaxInboundHandlers   int
	MaxInboundPerPeer    int
}

func DefaultFetchConfig() FetchConfig {
	return FetchConfig{Timeout: 10 * time.Second, PeerTimeout: 2 * time.Second, MaxPeers: DefaultMaxObjectFetchPeers,
		MaxConcurrentFetches: 8, MaxInboundHandlers: 8, MaxInboundPerPeer: 2}
}

func (c FetchConfig) validate() error {
	if c.Timeout <= 0 || c.Timeout > time.Minute || c.PeerTimeout <= 0 || c.PeerTimeout > c.Timeout ||
		c.MaxPeers < 1 || c.MaxPeers > HardMaxObjectFetchPeers ||
		c.MaxConcurrentFetches < 1 || c.MaxConcurrentFetches > 64 || c.MaxInboundHandlers < 1 || c.MaxInboundHandlers > 64 ||
		c.MaxInboundPerPeer < 1 || c.MaxInboundPerPeer > c.MaxInboundHandlers {
		return fmt.Errorf("invalid object fetch configuration")
	}
	return nil
}

type Service struct {
	node      *p2p.Node
	store     *Store
	networkID protocol.NetworkID
	cfg       FetchConfig
	ctx       context.Context
	cancel    context.CancelFunc
	outbound  chan struct{}
	inbound   chan struct{}
	requests  singleflight.Group

	peerMu         sync.Mutex
	peerActive     map[libpeer.ID]int
	candidatePeers func() []libpeer.ID
	lifecycleMu    sync.Mutex
	closed         bool
	wg             sync.WaitGroup
	closeOnce      sync.Once
}

func NewService(parent context.Context, node *p2p.Node, store *Store, networkID protocol.NetworkID, cfg FetchConfig) (*Service, error) {
	if parent == nil || node == nil || store == nil || networkID == "" {
		return nil, fmt.Errorf("object service dependencies are required")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Service{node: node, store: store, networkID: networkID, cfg: cfg, ctx: ctx, cancel: cancel,
		outbound: make(chan struct{}, cfg.MaxConcurrentFetches), inbound: make(chan struct{}, cfg.MaxInboundHandlers),
		peerActive: make(map[libpeer.ID]int)}
	s.candidatePeers = func() []libpeer.ID {
		remotes := s.node.UsablePeers()
		ids := make([]libpeer.ID, 0, len(remotes))
		for _, remote := range remotes {
			ids = append(ids, remote.PeerID)
		}
		return ids
	}
	node.Host().SetStreamHandler(ObjectProtocolID, s.handleRequest)
	return s, nil
}

func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.lifecycleMu.Lock()
		s.closed = true
		s.lifecycleMu.Unlock()
		s.cancel()
		s.node.Host().RemoveStreamHandler(ObjectProtocolID)
		s.wg.Wait()
	})
}

func (s *Service) FetchObject(ctx context.Context, id protocol.ObjectID) (Object, error) {
	if ctx == nil {
		return Object{}, fmt.Errorf("nil context")
	}
	if err := id.Validate(); err != nil || id.Algorithm != protocol.HashAlgorithmSHA256 {
		return Object{}, fmt.Errorf("invalid ObjectID")
	}
	select {
	case <-s.ctx.Done():
		return Object{}, s.ctx.Err()
	default:
	}
	object, err := s.store.Get(ctx, id)
	if err == nil {
		return object, nil
	}
	if !errors.Is(err, ErrObjectNotFound) {
		return Object{}, err
	}

	result := s.requests.DoChan(id.String(), func() (any, error) {
		if !s.beginWork() {
			return Object{}, context.Canceled
		}
		defer s.wg.Done()
		fetchCtx, cancel := context.WithTimeout(s.ctx, s.cfg.Timeout)
		defer cancel()
		return s.fetchNetwork(fetchCtx, id)
	})
	select {
	case <-ctx.Done():
		return Object{}, ctx.Err()
	case <-s.ctx.Done():
		return Object{}, s.ctx.Err()
	case outcome := <-result:
		if outcome.Err != nil {
			return Object{}, outcome.Err
		}
		return cloneObject(outcome.Val.(Object))
	}
}

func (s *Service) fetchNetwork(ctx context.Context, id protocol.ObjectID) (Object, error) {
	peers := s.candidates()
	if len(peers) == 0 {
		return Object{}, ErrNoAvailablePeers
	}
	var lastInvalid error
	sawNotFound := false
	sawTimeout := false
	for _, peerID := range peers {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return Object{}, ErrObjectFetchTimeout
			}
			return Object{}, err
		}
		object, status, err := s.requestPeer(ctx, peerID, id)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				sawTimeout = true
				continue
			}
			lastInvalid = err
			continue
		}
		if status == ObjectNotFound {
			sawNotFound = true
			continue
		}
		if status != ObjectFound {
			lastInvalid = fmt.Errorf("remote status %s", status)
			continue
		}
		storedID, _, err := s.store.Put(ctx, object)
		if err != nil {
			return Object{}, err
		}
		if storedID.String() != id.String() {
			return Object{}, ErrInvalidRemoteResponse
		}
		return object, nil
	}
	if lastInvalid != nil {
		return Object{}, fmt.Errorf("%w: %v", ErrInvalidRemoteResponse, lastInvalid)
	}
	if sawTimeout {
		return Object{}, ErrObjectFetchTimeout
	}
	if sawNotFound {
		return Object{}, ErrObjectNotFound
	}
	return Object{}, ErrNoAvailablePeers
}

func (s *Service) candidates() []libpeer.ID {
	candidates := append([]libpeer.ID(nil), s.candidatePeers()...)
	seen := make(map[libpeer.ID]struct{}, len(candidates))
	result := make([]libpeer.ID, 0, min(len(candidates), s.cfg.MaxPeers))
	for _, peerID := range candidates {
		if peerID == s.node.PeerID() || !s.node.IsUsable(peerID) {
			continue
		}
		if _, exists := seen[peerID]; exists {
			continue
		}
		seen[peerID] = struct{}{}
		result = append(result, peerID)
	}
	if len(result) > s.cfg.MaxPeers {
		result = result[:s.cfg.MaxPeers]
	}
	return result
}

func (s *Service) requestPeer(ctx context.Context, peerID libpeer.ID, id protocol.ObjectID) (Object, ObjectResponseStatus, error) {
	peerCtx, cancel := context.WithTimeout(ctx, s.cfg.PeerTimeout)
	defer cancel()
	select {
	case s.outbound <- struct{}{}:
		defer func() { <-s.outbound }()
	case <-peerCtx.Done():
		return Object{}, "", peerCtx.Err()
	}
	stream, err := s.node.Host().NewStream(peerCtx, peerID, ObjectProtocolID)
	if err != nil {
		return Object{}, "", err
	}
	streamDone := make(chan struct{})
	go func() {
		select {
		case <-peerCtx.Done():
			_ = stream.Reset()
		case <-streamDone:
		}
	}()
	defer close(streamDone)
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(s.cfg.PeerTimeout))
	request, err := EncodeObjectGetRequest(ObjectGetRequest{SchemaVersion: ObjectWireSchema, NetworkID: s.networkID, ObjectID: id.String()})
	if err != nil || writeObjectFrame(stream, request, MaxObjectRequestFrame) != nil {
		_ = stream.Reset()
		return Object{}, "", fmt.Errorf("send object request")
	}
	data, err := readObjectFrame(stream, MaxObjectResponseFrame)
	if err != nil {
		_ = stream.Reset()
		return Object{}, "", err
	}
	response, err := DecodeObjectGetResponse(data, s.networkID, id.String())
	if err != nil {
		return Object{}, "", err
	}
	if response.Status != ObjectFound {
		return Object{}, response.Status, nil
	}
	object, err := Decode(response.ObjectBytes)
	return object, response.Status, err
}

func (s *Service) handleRequest(stream libnetwork.Stream) {
	if !s.beginWork() {
		_ = stream.Reset()
		return
	}
	defer s.wg.Done()
	defer stream.Close()
	peerID := stream.Conn().RemotePeer()
	if !s.node.IsUsable(peerID) || !s.acquireInbound(peerID) {
		_ = stream.Reset()
		return
	}
	defer s.releaseInbound(peerID)
	_ = stream.SetDeadline(time.Now().Add(s.cfg.PeerTimeout))
	data, err := readObjectFrame(stream, MaxObjectRequestFrame)
	if err != nil {
		_ = stream.Reset()
		return
	}
	request, err := DecodeObjectGetRequest(data, s.networkID)
	if err != nil {
		_ = stream.Reset()
		return
	}
	id, _ := protocol.ParseObjectID(request.ObjectID)
	response := ObjectGetResponse{SchemaVersion: ObjectWireSchema, NetworkID: s.networkID, ObjectID: request.ObjectID, ObjectBytes: []byte{}}
	object, err := s.store.Get(s.ctx, id)
	switch {
	case err == nil:
		response.Status = ObjectFound
		response.ObjectBytes, err = object.CanonicalBytes()
	case errors.Is(err, ErrObjectNotFound):
		response.Status = ObjectNotFound
	case errors.Is(err, ErrObjectTooLarge):
		response.Status = ObjectTooLarge
	default:
		response.Status = ObjectStoreError
	}
	if err != nil && response.Status == ObjectFound {
		response.Status = ObjectStoreError
		response.ObjectBytes = []byte{}
	}
	encoded, err := EncodeObjectGetResponse(response)
	if err != nil || writeObjectFrame(stream, encoded, MaxObjectResponseFrame) != nil {
		_ = stream.Reset()
	}
}

func (s *Service) beginWork() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closed {
		return false
	}
	s.wg.Add(1)
	return true
}

func (s *Service) acquireInbound(peerID libpeer.ID) bool {
	select {
	case s.inbound <- struct{}{}:
	default:
		return false
	}
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if s.peerActive[peerID] >= s.cfg.MaxInboundPerPeer {
		<-s.inbound
		return false
	}
	s.peerActive[peerID]++
	return true
}

func (s *Service) releaseInbound(peerID libpeer.ID) {
	s.peerMu.Lock()
	s.peerActive[peerID]--
	if s.peerActive[peerID] == 0 {
		delete(s.peerActive, peerID)
	}
	s.peerMu.Unlock()
	<-s.inbound
}

func cloneObject(object Object) (Object, error) {
	data, err := object.CanonicalBytes()
	if err != nil {
		return Object{}, err
	}
	return Decode(data)
}
