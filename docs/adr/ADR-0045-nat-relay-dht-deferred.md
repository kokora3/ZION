# ADR-0045: NAT Traversal, Relay, and DHT Are Deferred

## Status

Accepted.

## Decision

Do not enable AutoNAT, hole punching, DCUtR, UPnP, NAT-PMP, circuit relay, TURN, DHT, or a required mDNS discovery path in Phase 7.

## Consequences

Discovery remains peer cache, bootstrap seeds, static fallback, manual peer, and bounded PEX. Reachable peers are still required as outbound targets.
