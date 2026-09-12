# ADR-0038: Research and Resource Governance Execution Deferred

## Status

Accepted.

## Context

Later phases will define Research and Resource Registry state, but Phase 6 needs stable proposal-kind names without inventing those transitions.

## Decision

Reserve RESEARCH_ADMISSION and RESOURCE_ADMISSION as typed names. Reject proposal construction/execution until their canonical registry handlers exist.

## Consequences

No Phase 11 registry state is introduced. A reserved name cannot be mistaken for an executable governance effect.
