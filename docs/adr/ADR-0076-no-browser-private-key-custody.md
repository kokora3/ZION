# ADR-0076: Private Member Keys Are Not Stored in Browser State

## Status

Accepted.

## Decision

Because the runtime exposes no safe general local member signer, Phase 12 supports already-signed transaction and Board-event submission. It never requests or persists a key or seed.

## Consequences

Interactive vote/proposal/publish builders remain unavailable until a separately reviewed local signer exists; the UI does not weaken custody to enable buttons.
