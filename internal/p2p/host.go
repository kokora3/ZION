package p2p

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	libhost "github.com/libp2p/go-libp2p/core/host"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	libprotocol "github.com/libp2p/go-libp2p/core/protocol"
	connmgr "github.com/libp2p/go-libp2p/p2p/net/connmgr"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/sync/singleflight"
)

type RemotePeer struct {
	PeerID    libpeer.ID
	Addresses []ma.Multiaddr
	Roles     []Role
	Version   Version
}

type Node struct {
	cfg          Config
	host         libhost.Host
	cache        *PeerCache
	cacheLoadErr error
	ctx          context.Context
	cancel       context.CancelFunc
	dialSem      chan struct{}
	mu           sync.RWMutex
	usable       map[libpeer.ID]RemotePeer
	closed       bool
	closeOnce    sync.Once
	gater        *connectionGater
	dials        singleflight.Group
}

func NewNode(parent context.Context, cfg Config) (*Node, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	key, err := LoadOrCreatePeerKey(cfg.KeyPath)
	if err != nil {
		return nil, err
	}
	cache, cacheErr := OpenPeerCache(cfg.PeerCachePath, cfg.Limits)
	if cache == nil {
		return nil, cacheErr
	}
	peerID, err := libpeer.IDFromPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("derive libp2p PeerID: %w", err)
	}
	gater := newConnectionGater(peerID, cfg.Limits.MaxConnectedPeers)
	manager, err := connmgr.NewConnManager(max(0, cfg.Limits.TargetOutboundPeers-1), cfg.Limits.MaxConnectedPeers)
	if err != nil {
		return nil, fmt.Errorf("create libp2p connection manager: %w", err)
	}
	options := []libp2p.Option{
		libp2p.Identity(key),
		libp2p.Transport(quic.NewTransport),
		libp2p.DisableRelay(),
		libp2p.DisableMetrics(),
		libp2p.ConnectionGater(gater),
		libp2p.ConnectionManager(manager),
	}
	if len(cfg.ListenAddresses) == 0 {
		options = append(options, libp2p.NoListenAddrs)
	} else {
		options = append(options, libp2p.ListenAddrStrings(cfg.ListenAddresses...))
	}
	if len(cfg.AdvertiseAddresses) > 0 {
		advertised := make([]ma.Multiaddr, 0, len(cfg.AdvertiseAddresses))
		for _, value := range cfg.AdvertiseAddresses {
			address, _ := ma.NewMultiaddr(value)
			advertised = append(advertised, address)
		}
		options = append(options, libp2p.AddrsFactory(func([]ma.Multiaddr) []ma.Multiaddr {
			return append([]ma.Multiaddr(nil), advertised...)
		}))
	}
	host, err := libp2p.New(options...)
	if err != nil {
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	node := &Node{cfg: cfg, host: host, cache: cache, cacheLoadErr: cacheErr, ctx: ctx, cancel: cancel,
		dialSem: make(chan struct{}, cfg.Limits.MaxConcurrentDials), usable: make(map[libpeer.ID]RemotePeer), gater: gater}
	host.SetStreamHandler(HelloProtocolID, node.handleHello)
	host.SetStreamHandler(PEXProtocolID, node.handlePEX)
	host.Network().Notify(&libnetwork.NotifyBundle{DisconnectedF: func(network libnetwork.Network, connection libnetwork.Conn) {
		gater.released(connection.RemotePeer())
		if len(network.ConnsToPeer(connection.RemotePeer())) == 0 {
			node.mu.Lock()
			delete(node.usable, connection.RemotePeer())
			node.mu.Unlock()
		}
	}})
	return node, nil
}

func (n *Node) Host() libhost.Host    { return n.host }
func (n *Node) PeerID() libpeer.ID    { return n.host.ID() }
func (n *Node) CacheLoadError() error { return n.cacheLoadErr }
func (n *Node) ListenAddresses() []ma.Multiaddr {
	return append([]ma.Multiaddr(nil), n.host.Network().ListenAddresses()...)
}

func (n *Node) AdvertisedAddresses() []ma.Multiaddr {
	return append([]ma.Multiaddr(nil), n.host.Addrs()...)
}

func (n *Node) FullAddresses() []ma.Multiaddr {
	component, _ := ma.NewMultiaddr("/p2p/" + n.PeerID().String())
	addresses := n.AdvertisedAddresses()
	result := make([]ma.Multiaddr, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, address.Encapsulate(component))
	}
	return result
}

func (n *Node) UsablePeers() []RemotePeer {
	n.mu.RLock()
	defer n.mu.RUnlock()
	peers := make([]RemotePeer, 0, len(n.usable))
	for _, remote := range n.usable {
		remote.Addresses = append([]ma.Multiaddr(nil), remote.Addresses...)
		remote.Roles = append([]Role(nil), remote.Roles...)
		peers = append(peers, remote)
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].PeerID.String() < peers[j].PeerID.String() })
	return peers
}

func (n *Node) IsUsable(peerID libpeer.ID) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	_, ok := n.usable[peerID]
	return ok
}

// ConnectionCounts returns bounded operational counts by authenticated PeerID.
// Duplicate transport connections never inflate logical peer metrics.
func (n *Node) ConnectionCounts() (connected, outbound int) {
	n.mu.RLock()
	usable := make(map[libpeer.ID]struct{}, len(n.usable))
	for id := range n.usable {
		usable[id] = struct{}{}
	}
	n.mu.RUnlock()
	connected = len(usable)
	seenOutbound := make(map[libpeer.ID]struct{}, connected)
	for _, connection := range n.host.Network().Conns() {
		id := connection.RemotePeer()
		if _, ok := usable[id]; ok && connection.Stat().Direction == libnetwork.DirOutbound {
			seenOutbound[id] = struct{}{}
		}
	}
	return connected, len(seenOutbound)
}

func (n *Node) localHello() Hello {
	return Hello{
		SchemaVersion:       WireSchema,
		NetworkID:           n.cfg.NetworkID,
		NetworkFingerprint:  n.cfg.NetworkFingerprint,
		SupportedVersions:   append([]Version(nil), n.cfg.SupportedVersions...),
		Roles:               append([]Role(nil), n.cfg.Roles...),
		AuthenticatedPeerID: n.PeerID().String(),
		AdvertisedAddresses: sortedAddressStrings(n.AdvertisedAddresses(), MaxAdvertisedAddresses),
	}
}

func (n *Node) validateRemoteHello(hello Hello, authenticated libpeer.ID) (Version, []ma.Multiaddr, error) {
	if err := validateHello(hello, authenticated); err != nil {
		return Version{}, nil, err
	}
	if hello.NetworkID != n.cfg.NetworkID {
		return Version{}, nil, fmt.Errorf("wrong ZION network")
	}
	if hello.NetworkFingerprint.Algorithm != n.cfg.NetworkFingerprint.Algorithm ||
		!bytes.Equal(hello.NetworkFingerprint.Digest, n.cfg.NetworkFingerprint.Digest) {
		return Version{}, nil, fmt.Errorf("wrong ZION network fingerprint")
	}
	version, err := negotiateVersion(n.cfg.SupportedVersions, hello.SupportedVersions)
	if err != nil {
		return Version{}, nil, err
	}
	addresses := make([]ma.Multiaddr, 0, len(hello.AdvertisedAddresses))
	for _, value := range hello.AdvertisedAddresses {
		address, _ := ma.NewMultiaddr(value)
		addresses = append(addresses, address)
	}
	return version, addresses, nil
}

func (n *Node) exchangeHello(stream libnetwork.Stream) (RemotePeer, error) {
	if err := stream.SetDeadline(time.Now().Add(n.cfg.Limits.HandshakeTimeout)); err != nil {
		return RemotePeer{}, err
	}
	defer stream.SetDeadline(time.Time{})
	localBytes, err := EncodeHello(n.localHello())
	if err != nil {
		return RemotePeer{}, err
	}
	if err := writeFrame(stream, localBytes, MaxHelloSize); err != nil {
		return RemotePeer{}, err
	}
	remoteBytes, err := readFrame(stream, MaxHelloSize)
	if err != nil {
		return RemotePeer{}, err
	}
	hello, err := DecodeHello(remoteBytes)
	if err != nil {
		return RemotePeer{}, err
	}
	remoteID := stream.Conn().RemotePeer()
	version, addresses, err := n.validateRemoteHello(hello, remoteID)
	if err != nil {
		return RemotePeer{}, err
	}
	roles, _ := normalizeRoles(hello.Roles)
	return RemotePeer{PeerID: remoteID, Addresses: addresses, Roles: roles, Version: version}, nil
}

func (n *Node) promote(remote RemotePeer, source PeerSource, fallbackAddress ma.Multiaddr) error {
	if remote.PeerID == n.PeerID() {
		return fmt.Errorf("self connection rejected")
	}
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return fmt.Errorf("P2P node closed")
	}
	if _, exists := n.usable[remote.PeerID]; !exists && len(n.usable) >= n.cfg.Limits.MaxConnectedPeers {
		n.mu.Unlock()
		return fmt.Errorf("maximum usable peers reached")
	}
	n.usable[remote.PeerID] = remote
	n.mu.Unlock()
	addresses := remote.Addresses
	if len(addresses) == 0 && fallbackAddress != nil {
		addresses = []ma.Multiaddr{fallbackAddress}
	}
	if err := n.cache.markSuccess(remote.PeerID, addresses, source); err != nil {
		n.mu.Lock()
		delete(n.usable, remote.PeerID)
		n.mu.Unlock()
		return err
	}
	return nil
}

func (n *Node) handleHello(stream libnetwork.Stream) {
	defer stream.Close()
	if err := stream.SetDeadline(time.Now().Add(n.cfg.Limits.HandshakeTimeout)); err != nil {
		n.rejectHello(stream)
		return
	}
	remoteBytes, err := readFrame(stream, MaxHelloSize)
	if err != nil {
		n.rejectHello(stream)
		return
	}
	hello, err := DecodeHello(remoteBytes)
	if err != nil {
		n.rejectHello(stream)
		return
	}
	remoteID := stream.Conn().RemotePeer()
	version, addresses, err := n.validateRemoteHello(hello, remoteID)
	if err != nil {
		n.rejectHello(stream)
		return
	}
	roles, _ := normalizeRoles(hello.Roles)
	remote := RemotePeer{PeerID: remoteID, Addresses: addresses, Roles: roles, Version: version}
	localBytes, err := EncodeHello(n.localHello())
	if err != nil || writeFrame(stream, localBytes, MaxHelloSize) != nil {
		n.rejectHello(stream)
		return
	}
	if err := n.promote(remote, SourceInbound, stream.Conn().RemoteMultiaddr()); err != nil {
		n.rejectHello(stream)
	}
}

func (n *Node) rejectHello(stream libnetwork.Stream) {
	remoteID := stream.Conn().RemotePeer()
	_ = stream.Reset()
	if !n.IsUsable(remoteID) {
		_ = n.host.Network().ClosePeer(remoteID)
	}
}

func (n *Node) Dial(ctx context.Context, address string, source PeerSource) error {
	info, err := parseDialAddress(address)
	if err != nil {
		return err
	}
	return n.dialInfo(ctx, *info, source)
}

func parseDialAddress(value string) (*libpeer.AddrInfo, error) {
	if len(value) == 0 || len(value) > MaxMultiaddrSize {
		return nil, fmt.Errorf("invalid dial multiaddr size")
	}
	address, err := ma.NewMultiaddr(value)
	if err != nil {
		return nil, fmt.Errorf("invalid dial multiaddr: %w", err)
	}
	info, err := libpeer.AddrInfoFromP2pAddr(address)
	if err != nil {
		return nil, fmt.Errorf("dial multiaddr must include expected /p2p/PeerID: %w", err)
	}
	return info, nil
}

func (n *Node) dialInfo(ctx context.Context, info libpeer.AddrInfo, source PeerSource) error {
	result := n.dials.DoChan(info.ID.String(), func() (any, error) {
		return nil, n.dialInfoOnce(ctx, info, source)
	})
	select {
	case outcome := <-result:
		return outcome.Err
	case <-ctx.Done():
		return ctx.Err()
	case <-n.ctx.Done():
		return n.ctx.Err()
	}
}

func (n *Node) dialInfoOnce(ctx context.Context, info libpeer.AddrInfo, source PeerSource) error {
	if info.ID == n.PeerID() {
		return fmt.Errorf("self connection rejected")
	}
	if n.IsUsable(info.ID) {
		return nil
	}
	if n.cache.backoffActive(info.ID, info.Addrs) {
		return fmt.Errorf("dial backoff active for peer %s", info.ID)
	}
	select {
	case n.dialSem <- struct{}{}:
		defer func() { <-n.dialSem }()
	case <-ctx.Done():
		return ctx.Err()
	case <-n.ctx.Done():
		return n.ctx.Err()
	}
	dialCtx, cancel := context.WithTimeout(ctx, n.cfg.Limits.DialTimeout)
	defer cancel()
	if err := n.host.Connect(dialCtx, info); err != nil {
		_ = n.cache.markFailure(info.ID, info.Addrs, source)
		return fmt.Errorf("authenticate libp2p peer %s: %w", info.ID, err)
	}
	streamCtx, streamCancel := context.WithTimeout(dialCtx, n.cfg.Limits.HandshakeTimeout)
	defer streamCancel()
	stream, err := n.host.NewStream(streamCtx, info.ID, HelloProtocolID)
	if err != nil {
		_ = n.cache.markFailure(info.ID, info.Addrs, source)
		_ = n.host.Network().ClosePeer(info.ID)
		return fmt.Errorf("open ZION hello: %w", err)
	}
	remote, err := n.exchangeHello(stream)
	_ = stream.Close()
	if err != nil {
		_ = n.cache.markFailure(info.ID, info.Addrs, source)
		_ = n.host.Network().ClosePeer(info.ID)
		return fmt.Errorf("ZION hello rejected: %w", err)
	}
	fallback := firstAddress(info.Addrs)
	if err := n.promote(remote, source, fallback); err != nil {
		_ = n.host.Network().ClosePeer(info.ID)
		return err
	}
	return nil
}

func firstAddress(addresses []ma.Multiaddr) ma.Multiaddr {
	if len(addresses) == 0 {
		return nil
	}
	return addresses[0]
}

func (n *Node) Close() error {
	var result error
	n.closeOnce.Do(func() {
		n.mu.Lock()
		n.closed = true
		n.mu.Unlock()
		n.cancel()
		n.host.RemoveStreamHandler(HelloProtocolID)
		n.host.RemoveStreamHandler(PEXProtocolID)
		if err := n.cache.Flush(); err != nil {
			result = err
		}
		if err := n.host.Close(); result == nil {
			result = err
		}
	})
	return result
}

var _ libprotocol.ID = HelloProtocolID
