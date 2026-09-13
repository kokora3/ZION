# ADR-0052: Operational Runtime State Is Separate from Canonical State

## Status

Accepted.

## Decision

Keep lifecycle, sync status, API config, PeerID/cache, accepted-height metadata, and recent transaction status outside canonical chain state.

## Consequences

Operational changes cannot alter StateHash or AppHash.
