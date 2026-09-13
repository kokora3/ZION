# ADR-0067: ResearchID and ResourceID Are Immutable Content-Derived IDs

## Status

Accepted.

## Decision

Derive separately typed SHA-256 IDs from validated canonical CBOR entry bodies, excluding the ID and admission provenance from the preimage.

## Consequences

Equal bodies have equal IDs, changed bodies have new IDs, and no self-hash cycle exists.
