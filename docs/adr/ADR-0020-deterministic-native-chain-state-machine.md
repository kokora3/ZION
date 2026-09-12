# ADR-0020: Deterministic Native Chain State Machine

## Status
Accepted.

## Context
ZION needs one state transition result for each finalized transaction order without coupling protocol state to a particular consensus implementation.

## Decision
Implement the Phase 4 native chain as a pure ordered transition boundary whose consensus inputs are previous canonical state and one transaction. Exclude consensus, blocks, networking, persistence, local clocks, randomness, filesystem state, and external services.

## Consequences
Future consensus code may order transactions but must not reinterpret transitions. Deterministic replay can validate the state machine independently of consensus.
