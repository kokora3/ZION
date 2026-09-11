# ADR-0004: Keep payload content off-chain

## Status

Accepted for Phase 1.

## Decision

The chain stores compact trusted state, registries, governance decisions, and references; payload content remains off-chain.

## Consequences

Full PDFs, repositories, large datasets, large binaries, and large attachments do not belong in chain state.
