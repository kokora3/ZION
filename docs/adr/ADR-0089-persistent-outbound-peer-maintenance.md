# ADR-0089: Maintain Known Outbound Peers for the Runtime Lifetime

## Status

Accepted for the D2B reconnect hotfix.

## Context

Startup discovery successfully connected an outbound-only Windows NORMAL node to the public bootstrap. After the bootstrap host restarted, libp2p correctly removed the closed peer, but discovery had already returned and no lifecycle worker invoked it again. The independent state-sync retry loop only iterated currently usable peers and therefore could not restore transport connectivity.

## Decision

Run one cancellable outbound-maintenance worker per general-P2P node. Preserve the existing source order—successful cache, configured bootstrap, fallback, manual, then bounded PEX—and wake maintenance when a usable peer's final connection closes. Retry while authenticated outbound connections are below `target_outbound_peers` using the peer cache's bounded exponential failure state with jitter. Continue to route every attempt through PeerID-bearing multiaddrs, libp2p authentication, ZION Hello compatibility checks, per-PeerID singleflight, and the existing dial semaphore.

Configured bootstrap addresses remain immutable runtime candidates even after success or temporary failure. A successful reconnect clears its failure count and next-attempt time. Node cancellation stops the maintenance worker and timer before host shutdown.

## Consequences

An outbound-only NORMAL node reconnects after a public bootstrap restart without a local process restart or inbound listener. DNS can change location on a later dial but cannot change the expected PeerID. Bootstrap remains replaceable non-authority. This hotfix changes no protocol ID, wire message, canonical encoding, chain identity, or consensus behavior.
