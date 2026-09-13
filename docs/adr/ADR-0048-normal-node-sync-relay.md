# ADR-0048: Normal Nodes Use General P2P State Sync and Transaction Relay

## Status

Accepted.

## Decision

Use versioned bounded /zion/state/0.1.0 and /zion/tx/0.1.0 protocols over authenticated Phase 7 peers.

## Consequences

Normal nodes need no validator or inbound reachability. Relay acceptance is not finality.
