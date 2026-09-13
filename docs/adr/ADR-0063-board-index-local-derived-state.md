# ADR-0063: Board Index and Search Are Local Rebuildable Derived State

## Status

Accepted.

## Decision

Feed, thread, and search metadata live in a bounded local JSON index rebuildable from verified immutable Board event objects.

## Consequences

Index corruption or deletion does not affect canonical chain state. Search behavior may evolve locally without changing ObjectID, StateHash, or AppHash.
