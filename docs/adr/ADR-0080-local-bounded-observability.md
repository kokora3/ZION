# ADR-0080: Local Observability Is Non-Canonical and Bounded

## Status

Accepted.

## Decision

Emit structured JSON operational logs and expose Prometheus text at `/metrics` on the same listener, bearer-authentication, CORS, concurrency, and timeout boundary as the local API. Metrics use a fixed name and label set; per-peer, per-identity, per-transaction, per-object, and per-post labels are forbidden.

## Consequences

Operators can diagnose a node locally without central telemetry. Timestamps, counters, build metadata, log level, and metrics configuration never enter canonical Apply, Snapshot, TxID, ObjectID, StateHash, or AppHash.
