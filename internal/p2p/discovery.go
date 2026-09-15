package p2p

import (
	"context"
	"fmt"

	libpeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Discover follows the normative order: authenticated cache, configured/DNS
// bootstrap seeds, static fallback seeds, then manual peers.
func (n *Node) Discover(ctx context.Context) error {
	groups := []struct {
		source    PeerSource
		addresses []string
	}{
		{SourceCache, cachedAddresses(n.cache.SuccessfulCandidates(n.PeerID()))},
		{SourceBootstrap, append([]string(nil), n.cfg.BootstrapAddresses...)},
		{SourceFallback, append([]string(nil), n.cfg.FallbackAddresses...)},
		{SourceManual, append([]string(nil), n.cfg.ManualPeers...)},
	}
	var lastErr error
	connectedAny := false
	for _, group := range groups {
		for _, address := range group.addresses {
			if err := n.Dial(ctx, address, group.source); err != nil {
				lastErr = err
				continue
			}
			connectedAny = true
			info, _ := parseDialAddress(address)
			if info != nil {
				candidates, err := n.RequestPeers(ctx, info.ID)
				if err == nil {
					n.dialPEXHints(ctx, candidates)
				}
			}
			if n.outboundTargetReached() {
				return nil
			}
		}
	}
	if connectedAny {
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no P2P discovery candidates")
	}
	return lastErr
}

func cachedAddresses(peers []CachedPeer) []string {
	result := make([]string, 0)
	for _, record := range peers {
		component, err := ma.NewMultiaddr("/p2p/" + record.PeerID)
		if err != nil {
			continue
		}
		for _, value := range record.Addresses {
			address, err := ma.NewMultiaddr(value)
			if err == nil {
				result = append(result, address.Encapsulate(component).String())
			}
		}
	}
	return result
}

func (n *Node) dialPEXHints(ctx context.Context, records []PeerRecord) {
	for _, record := range records {
		peerID, err := libpeer.Decode(record.PeerID)
		if err != nil || peerID == n.PeerID() || n.IsUsable(peerID) {
			continue
		}
		addresses := make([]ma.Multiaddr, 0, len(record.Addresses))
		for _, value := range record.Addresses {
			address, err := ma.NewMultiaddr(value)
			if err == nil {
				addresses = append(addresses, address)
			}
		}
		_ = n.dialInfo(ctx, libpeer.AddrInfo{ID: peerID, Addrs: addresses}, SourcePEX)
		if n.outboundTargetReached() {
			return
		}
	}
}
