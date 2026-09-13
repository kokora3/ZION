package board

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/kokora3/zion/internal/p2p"
	"github.com/kokora3/zion/internal/protocol"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	libprotocol "github.com/libp2p/go-libp2p/core/protocol"
)

type Config struct {
	Enabled               bool
	AnnounceFanout        int
	SyncInterval          time.Duration
	SyncPageSize          int
	MaxSyncEventsPerCycle int
	MaxConcurrentHandlers int
	PeerTimeout           time.Duration
}

func DefaultConfig() Config {
	return Config{Enabled: true, AnnounceFanout: 8, SyncInterval: 30 * time.Second, SyncPageSize: 16,
		MaxSyncEventsPerCycle: 256, MaxConcurrentHandlers: 8, PeerTimeout: 3 * time.Second}
}

func (c Config) validate() error {
	if c.AnnounceFanout < 1 || c.AnnounceFanout > 16 || c.SyncInterval < 100*time.Millisecond || c.SyncInterval > time.Hour ||
		c.SyncPageSize < 1 || c.SyncPageSize > MaxSyncPageEvents || c.MaxSyncEventsPerCycle < c.SyncPageSize ||
		c.MaxSyncEventsPerCycle > 4096 || c.MaxConcurrentHandlers < 1 || c.MaxConcurrentHandlers > 64 ||
		c.PeerTimeout <= 0 || c.PeerTimeout > 30*time.Second {
		return fmt.Errorf("invalid board configuration")
	}
	return nil
}

type Repository interface {
	AdmitRemoteBoardEvent(context.Context, []byte) error
	BoardInventory(context.Context, string, int) (events [][]byte, next string, more bool, err error)
}

type Service struct {
	ctx    context.Context
	cancel context.CancelFunc
	node   *p2p.Node
	net    protocol.NetworkID
	cfg    Config
	repo   Repository
	sem    chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
	lifeMu sync.Mutex
	closed bool
}

func NewService(parent context.Context, node *p2p.Node, network protocol.NetworkID, cfg Config, repo Repository) (*Service, error) {
	if parent == nil || node == nil || network == "" || repo == nil {
		return nil, fmt.Errorf("board service dependencies are required")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Service{ctx: ctx, cancel: cancel, node: node, net: network, cfg: cfg, repo: repo, sem: make(chan struct{}, cfg.MaxConcurrentHandlers)}
	node.Host().SetStreamHandler(libprotocol.ID(AnnounceProtocolID), s.handleAnnounce)
	node.Host().SetStreamHandler(libprotocol.ID(SyncProtocolID), s.handleSync)
	s.wg.Add(1)
	go s.loop()
	return s, nil
}

func (s *Service) Close() {
	s.once.Do(func() {
		s.lifeMu.Lock()
		s.closed = true
		s.lifeMu.Unlock()
		s.cancel()
		s.node.Host().RemoveStreamHandler(libprotocol.ID(AnnounceProtocolID))
		s.node.Host().RemoveStreamHandler(libprotocol.ID(SyncProtocolID))
		s.wg.Wait()
	})
}

func (s *Service) Announce(ctx context.Context, eventObject []byte) error {
	message, err := EncodeAnnounce(Announce{SchemaVersion: BoardWireSchema, NetworkID: s.net, EventObject: eventObject})
	if err != nil {
		return err
	}
	peers := s.peers(s.cfg.AnnounceFanout)
	for _, peerID := range peers {
		peerCtx, cancel := context.WithTimeout(ctx, s.cfg.PeerTimeout)
		stream, err := s.node.Host().NewStream(peerCtx, peerID, libprotocol.ID(AnnounceProtocolID))
		if err == nil {
			_ = stream.SetDeadline(time.Now().Add(s.cfg.PeerTimeout))
			err = writeFrame(stream, message, MaxAnnounceFrame)
			_ = stream.Close()
		}
		cancel()
	}
	return nil
}

func (s *Service) SyncNow(ctx context.Context) error {
	for _, peerID := range s.peers(4) {
		after, processed := "", 0
		for processed < s.cfg.MaxSyncEventsPerCycle {
			response, err := s.requestPage(ctx, peerID, after)
			if err != nil {
				break
			}
			for _, event := range response.Events {
				_ = s.repo.AdmitRemoteBoardEvent(ctx, event)
				processed++
				if processed >= s.cfg.MaxSyncEventsPerCycle {
					break
				}
			}
			if !response.More || response.NextPostID == "" || response.NextPostID <= after {
				break
			}
			after = response.NextPostID
		}
	}
	return ctx.Err()
}

func (s *Service) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, s.cfg.PeerTimeout*4)
			_ = s.SyncNow(ctx) // Every cycle deliberately rescans from the start.
			cancel()
		}
	}
}

func (s *Service) requestPage(ctx context.Context, peerID libpeer.ID, after string) (SyncResponse, error) {
	peerCtx, cancel := context.WithTimeout(ctx, s.cfg.PeerTimeout)
	defer cancel()
	stream, err := s.node.Host().NewStream(peerCtx, peerID, libprotocol.ID(SyncProtocolID))
	if err != nil {
		return SyncResponse{}, err
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(s.cfg.PeerTimeout))
	request, err := EncodeSyncRequest(SyncRequest{SchemaVersion: BoardWireSchema, NetworkID: s.net, AfterPostID: after, Limit: uint16(s.cfg.SyncPageSize)})
	if err != nil || writeFrame(stream, request, MaxSyncRequestFrame) != nil {
		_ = stream.Reset()
		return SyncResponse{}, fmt.Errorf("board sync request failed")
	}
	data, err := readFrame(stream, MaxSyncResponseFrame)
	if err != nil {
		return SyncResponse{}, err
	}
	return DecodeSyncResponse(data, s.net)
}

func (s *Service) handleAnnounce(stream libnetwork.Stream) {
	if !s.acquire() {
		_ = stream.Reset()
		return
	}
	defer s.release()
	defer stream.Close()
	if !s.node.IsUsable(stream.Conn().RemotePeer()) {
		_ = stream.Reset()
		return
	}
	_ = stream.SetDeadline(time.Now().Add(s.cfg.PeerTimeout))
	data, err := readFrame(stream, MaxAnnounceFrame)
	if err != nil {
		_ = stream.Reset()
		return
	}
	message, err := DecodeAnnounce(data, s.net)
	if err != nil || s.repo.AdmitRemoteBoardEvent(s.ctx, message.EventObject) != nil {
		_ = stream.Reset()
	}
}

func (s *Service) handleSync(stream libnetwork.Stream) {
	if !s.acquire() {
		_ = stream.Reset()
		return
	}
	defer s.release()
	defer stream.Close()
	if !s.node.IsUsable(stream.Conn().RemotePeer()) {
		_ = stream.Reset()
		return
	}
	_ = stream.SetDeadline(time.Now().Add(s.cfg.PeerTimeout))
	data, err := readFrame(stream, MaxSyncRequestFrame)
	if err != nil {
		_ = stream.Reset()
		return
	}
	request, err := DecodeSyncRequest(data, s.net)
	if err != nil {
		_ = stream.Reset()
		return
	}
	events, next, more, err := s.repo.BoardInventory(s.ctx, request.AfterPostID, int(request.Limit))
	if err != nil {
		_ = stream.Reset()
		return
	}
	response, err := EncodeSyncResponse(SyncResponse{SchemaVersion: BoardWireSchema, NetworkID: s.net, Events: events, NextPostID: next, More: more})
	if err != nil || writeFrame(stream, response, MaxSyncResponseFrame) != nil {
		_ = stream.Reset()
	}
}

func (s *Service) acquire() bool {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.sem <- struct{}{}:
		s.wg.Add(1)
		return true
	default:
		return false
	}
}
func (s *Service) release() { <-s.sem; s.wg.Done() }

func (s *Service) peers(maximum int) []libpeer.ID {
	values := s.node.UsablePeers()
	result := make([]libpeer.ID, 0, min(maximum, len(values)))
	for _, value := range values {
		if value.PeerID != s.node.PeerID() {
			result = append(result, value.PeerID)
			if len(result) == maximum {
				break
			}
		}
	}
	return result
}

func writeFrame(writer io.Writer, data []byte, maximum int) error {
	if len(data) == 0 || len(data) > maximum {
		return fmt.Errorf("invalid board frame size")
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	if _, err := writer.Write(header); err != nil {
		return err
	}
	_, err := writer.Write(data)
	return err
}

func readFrame(reader io.Reader, maximum int) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header)
	if size == 0 || size > uint32(maximum) {
		return nil, fmt.Errorf("invalid board frame size")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(reader, data)
	return data, err
}
