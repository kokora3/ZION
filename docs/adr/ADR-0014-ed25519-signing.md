# ADR-0014: Ed25519 as the Initial Signing Algorithm

## Status
Accepted.

## Context
ZION needs an auditable v0.1 signing implementation.

## Decision
Use Go standard-library Ed25519 with explicit `ed25519` algorithm identifiers.

## Consequences
Unknown algorithms fail closed; later versions may add identifiers.
