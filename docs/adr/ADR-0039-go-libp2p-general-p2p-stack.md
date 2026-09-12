# ADR-0039: go-libp2p Is the General P2P Reference Stack

## Status

Accepted.

## Decision

Pin go-libp2p v0.49.0 under its MIT license for ZION general networking. Enable QUIC v1 explicitly and use libp2p transport authentication rather than custom cryptography.

## Consequences

CometBFT networking remains the independent BFT transport. ZION owns only its versioned hello and peer-exchange messages.
