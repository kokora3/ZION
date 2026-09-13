# ADR-0053: Reuse Frozen Phase 2 ObjectID for Stored Content

## Status

Accepted.

## Decision

Phase 9 storage and retrieval use the Phase 2 `ObjectID` syntax and SHA-256 derivation unchanged. The ID is derived from canonical `UnsignedObjectCore`, excludes itself, and remains distinct from the payload `ContentHash`.

## Consequences

Stored and remotely retrieved bytes interoperate with frozen Phase 2 vectors. Phase 9 cannot alter the core encoding or derive an ID from a filename, path, peer, or storage location.
