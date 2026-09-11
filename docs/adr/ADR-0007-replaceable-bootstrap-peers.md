# ADR-0007: Bootstrap peers are replaceable discovery infrastructure

## Status

Accepted for Phase 1.

## Decision

Bootstrap peers help a node discover the network but are not consensus authority or an application source of truth.

## Consequences

Bootstrap addresses must be replaceable discovery configuration and must not become immutable chain state. Endpoints should include cryptographic peer identity.
