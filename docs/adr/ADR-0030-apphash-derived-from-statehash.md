# ADR-0030: CometBFT AppHash Derived from ZION StateHash

## Status
Accepted.

## Context
An unrelated consensus application hash would create conflicting commitments to application state.

## Decision
Set `AppHash` to the exact 32-byte SHA-256 digest contained in the Phase 4 `StateHash` of the canonical snapshot.

## Consequences
Identical ZION state necessarily yields identical CometBFT application commitments. Consensus height remains outside canonical state.
