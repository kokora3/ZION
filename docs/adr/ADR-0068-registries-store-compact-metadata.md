# ADR-0068: Registries Store Compact Metadata and References

## Status

Accepted.

## Decision

Canonical registries store bounded descriptive metadata, typed references, and governance provenance within the existing 4,096-byte proposal-payload limit.

## Consequences

Large payloads, repository contents, PDFs, and chunking remain outside canonical state.
