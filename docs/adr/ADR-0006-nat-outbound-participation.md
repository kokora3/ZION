# ADR-0006: NORMAL nodes may participate from behind NAT

## Status

Accepted for Phase 1.

## Decision

A NORMAL v0.1 node may participate by initiating outbound connections to reachable peers; inbound reachability is not required.

## Consequences

Hole punching, AutoNAT, DCUtR, and relay infrastructure remain deferred. Validators and bootstrap nodes should normally be publicly reachable.
