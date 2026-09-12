# ADR-0031: Validator-Set Changes Deferred Until Governance Authorization

## Status
Accepted.

## Context
Phase 5 defines an initial validator set but no governance authorization for changing it.

## Decision
Bind validators at genesis and emit no runtime validator updates. Local configuration cannot alter an established set.

## Consequences
Permissioned alpha consensus is available without inventing governance, staking, or token voting. Runtime changes require a future authorized protocol decision.
