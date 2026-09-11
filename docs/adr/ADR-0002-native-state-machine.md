# ADR-0002: Use a native ZION state machine

## Status

Accepted for Phase 1.

## Decision

ZION owns its transaction model, deterministic state-transition rules, validator authorization rules, chain data model, governance model, registry model, and node implementation rather than using a Cosmos or application-chain framework.

## Consequences

This does not authorize a novel consensus algorithm; a known BFT design remains required.
