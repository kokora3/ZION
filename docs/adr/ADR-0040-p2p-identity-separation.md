# ADR-0040: P2P PeerID Is Separate from Member and Validator Identities

## Status

Accepted.

## Decision

Generate and persist an independent libp2p key. Do not derive or reuse member or validator keys, and do not place the P2P private key or PeerID in canonical state.

## Consequences

P2P key replacement changes PeerID only. IdentityID, membership, governance identity, validator identity, StateHash, and AppHash remain unchanged.
