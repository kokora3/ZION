package p2p

import (
	"context"
	"time"

	libpeer "github.com/libp2p/go-libp2p/core/peer"
)

const outboundMaintenanceIdleInterval = 5 * time.Minute

func (n *Node) startOutboundMaintenance() {
	n.maintenanceOnce.Do(func() {
		n.maintenanceWG.Add(1)
		go func() {
			defer n.maintenanceWG.Done()
			n.maintainOutbound()
		}()
	})
}

func (n *Node) maintainOutbound() {
	for {
		if n.ctx.Err() != nil {
			return
		}
		if n.outboundTargetReached() || !n.hasKnownDialCandidate() {
			if !n.waitForMaintenance(0) {
				return
			}
			continue
		}

		_, before := n.ConnectionCounts()
		n.maintenanceAttempts.Add(1)
		roundCtx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
		err := n.Discover(roundCtx)
		cancel()
		_, after := n.ConnectionCounts()
		if after > before {
			n.maintenanceSuccesses.Add(uint64(after - before))
			n.cfg.Logger.Info("outbound peer reconnected", "outbound_peers", after,
				"target_outbound_peers", n.cfg.Limits.TargetOutboundPeers)
		}
		if after >= n.cfg.Limits.TargetOutboundPeers {
			n.cfg.Logger.Info("outbound peer target restored", "outbound_peers", after,
				"target_outbound_peers", n.cfg.Limits.TargetOutboundPeers)
			continue
		}
		delay := n.nextMaintenanceDelay()
		if err != nil {
			n.cfg.Logger.Warn("outbound peer redial failed", "error", err, "outbound_peers", after,
				"target_outbound_peers", n.cfg.Limits.TargetOutboundPeers, "retry_after", delay)
		}
		if !n.waitForMaintenance(delay) {
			return
		}
	}
}

func (n *Node) outboundTargetReached() bool {
	_, outbound := n.ConnectionCounts()
	return outbound >= n.cfg.Limits.TargetOutboundPeers
}

func (n *Node) hasKnownDialCandidate() bool {
	if len(n.cfg.BootstrapAddresses)+len(n.cfg.FallbackAddresses)+len(n.cfg.ManualPeers) > 0 {
		return true
	}
	connected := n.usablePeerSet()
	return n.cache.hasCandidate(n.PeerID(), connected)
}

func (n *Node) nextMaintenanceDelay() time.Duration {
	connected := n.usablePeerSet()
	if delay, ok := n.cache.nextAttemptDelay(n.PeerID(), connected); ok {
		return max(delay, time.Millisecond)
	}
	return min(outboundMaintenanceIdleInterval, n.cfg.Limits.BackoffMaximum)
}

func (n *Node) usablePeerSet() map[libpeer.ID]struct{} {
	n.mu.RLock()
	defer n.mu.RUnlock()
	result := make(map[libpeer.ID]struct{}, len(n.usable))
	for peerID := range n.usable {
		result[peerID] = struct{}{}
	}
	return result
}

func (n *Node) waitForMaintenance(delay time.Duration) bool {
	if delay <= 0 {
		select {
		case <-n.ctx.Done():
			return false
		case <-n.maintenanceWake:
			return true
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-n.ctx.Done():
		return false
	case <-n.maintenanceWake:
		return true
	case <-timer.C:
		return true
	}
}

func (n *Node) wakeMaintenance() {
	select {
	case n.maintenanceWake <- struct{}{}:
	default:
	}
}
