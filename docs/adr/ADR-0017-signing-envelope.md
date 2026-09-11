# ADR-0017: Domain-Separated Signing Envelopes

## Status
Accepted.

## Context
Signatures must not be replayed across operations.

## Decision
Sign canonical envelopes carrying domain, schema, network, purpose, and payload.

## Consequences
Network-scoped signatures bind NetworkID.
