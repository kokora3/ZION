package p2p

import (
	"sync"

	libconnmgr "github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/control"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// connectionGater enforces a hard bound on authenticated transport
// connections, including peers that never complete the ZION hello.
type connectionGater struct {
	mu     sync.Mutex
	self   libpeer.ID
	max    int
	total  int
	counts map[libpeer.ID]int
}

func newConnectionGater(self libpeer.ID, maximum int) *connectionGater {
	return &connectionGater{self: self, max: maximum, counts: make(map[libpeer.ID]int)}
}

func (g *connectionGater) InterceptPeerDial(peerID libpeer.ID) bool { return peerID != g.self }

func (g *connectionGater) InterceptAddrDial(libpeer.ID, ma.Multiaddr) bool { return true }

func (g *connectionGater) InterceptAccept(libnetwork.ConnMultiaddrs) bool { return true }

func (g *connectionGater) InterceptSecured(_ libnetwork.Direction, peerID libpeer.ID, _ libnetwork.ConnMultiaddrs) bool {
	return peerID != g.self
}

func (g *connectionGater) InterceptUpgraded(connection libnetwork.Conn) (bool, control.DisconnectReason) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if connection.RemotePeer() == g.self || g.total >= g.max {
		return false, 0
	}
	g.total++
	g.counts[connection.RemotePeer()]++
	return true, 0
}

func (g *connectionGater) released(peerID libpeer.ID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	count, exists := g.counts[peerID]
	if !exists {
		return
	}
	if count <= 1 {
		delete(g.counts, peerID)
	} else {
		g.counts[peerID] = count - 1
	}
	if g.total > 0 {
		g.total--
	}
}

var _ libconnmgr.ConnectionGater = (*connectionGater)(nil)
