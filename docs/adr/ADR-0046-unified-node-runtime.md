# ADR-0046: Unified zion-node Runtime Composition

## Status

Accepted.

## Decision

Compose existing protocol packages behind one explicit cancellable lifecycle. Do not duplicate chain or governance rules in runtime code.

## Consequences

Partial startup is cleaned up and role capabilities can overlap.
