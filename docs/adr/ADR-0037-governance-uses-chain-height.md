# ADR-0037: Governance Uses Chain Height Rather Than Wall Clock

## Status

Accepted.

## Context

Validator wall clocks are not deterministic state-machine inputs.

## Decision

Store proposal start and end as consensus heights. Voting is allowed before end height, and an ACTIVE member may trigger explicit deterministic finalization at or after end height.

## Consequences

All validators use the same lifecycle boundary. Local time, timers, cron jobs, and process scheduling cannot decide governance state.
