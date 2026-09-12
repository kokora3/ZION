package p2p

import "time"

const (
	MaxHelloSize               = 16 * 1024
	MaxPEXRequestSize          = 1024
	MaxPEXResponseSize         = 64 * 1024
	MaxRoles                   = 3
	MaxProtocolVersions        = 8
	MaxAdvertisedAddresses     = 8
	MaxMultiaddrSize           = 512
	HardMaxConnectedPeers      = 256
	HardMaxConcurrentDials     = 32
	HardMaxCachedPeers         = 1024
	HardMaxAddressesPerPeer    = 16
	HardMaxPEXPeersPerResponse = 64
	MaxPeerKeyFileSize         = 4096
	MaxPeerCacheFileSize       = 2 * 1024 * 1024
)

type Limits struct {
	MaxConnectedPeers      int
	TargetOutboundPeers    int
	MaxConcurrentDials     int
	MaxCachedPeers         int
	MaxAddressesPerPeer    int
	MaxPEXPeersPerResponse int
	DialTimeout            time.Duration
	HandshakeTimeout       time.Duration
	PEXTimeout             time.Duration
	BackoffInitial         time.Duration
	BackoffMaximum         time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		MaxConnectedPeers:      64,
		TargetOutboundPeers:    12,
		MaxConcurrentDials:     8,
		MaxCachedPeers:         256,
		MaxAddressesPerPeer:    8,
		MaxPEXPeersPerResponse: 32,
		DialTimeout:            10 * time.Second,
		HandshakeTimeout:       5 * time.Second,
		PEXTimeout:             5 * time.Second,
		BackoffInitial:         2 * time.Second,
		BackoffMaximum:         5 * time.Minute,
	}
}

func (l Limits) validate() error {
	if l.MaxConnectedPeers < 1 || l.MaxConnectedPeers > HardMaxConnectedPeers {
		return errLimit("connected peers", l.MaxConnectedPeers)
	}
	if l.TargetOutboundPeers < 1 || l.TargetOutboundPeers > l.MaxConnectedPeers {
		return errLimit("target outbound peers", l.TargetOutboundPeers)
	}
	if l.MaxConcurrentDials < 1 || l.MaxConcurrentDials > HardMaxConcurrentDials {
		return errLimit("concurrent dials", l.MaxConcurrentDials)
	}
	if l.MaxCachedPeers < 1 || l.MaxCachedPeers > HardMaxCachedPeers {
		return errLimit("cached peers", l.MaxCachedPeers)
	}
	if l.MaxAddressesPerPeer < 1 || l.MaxAddressesPerPeer > HardMaxAddressesPerPeer {
		return errLimit("addresses per peer", l.MaxAddressesPerPeer)
	}
	if l.MaxPEXPeersPerResponse < 1 || l.MaxPEXPeersPerResponse > HardMaxPEXPeersPerResponse {
		return errLimit("PEX peers", l.MaxPEXPeersPerResponse)
	}
	if l.DialTimeout <= 0 || l.HandshakeTimeout <= 0 || l.PEXTimeout <= 0 || l.BackoffInitial <= 0 || l.BackoffMaximum < l.BackoffInitial {
		return errLimit("timeout/backoff", 0)
	}
	return nil
}
