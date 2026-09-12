# ADR-0027: Consensus Engine Separate from Native ZION State Machine

## Status
Accepted.

## Context
Consensus ordering must not create a second path for identity or membership mutation.

## Decision
Use an ABCI adapter under `internal/consensus`; every canonical state transition executes through Phase 4 `chain.Apply`.

## Consequences
CometBFT can be integrated without changing frozen transactions, receipts, snapshots, or application rules. Check and proposal evaluation use temporary state only.
