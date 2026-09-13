# ADR-0054: Small Immutable Filesystem-Backed Object Store

## Status

Accepted.

## Decision

Store validated canonical objects in SHA-256 digest-sharded filesystem paths. The alpha hard limit is 1 MiB per canonical object. Writes use owner-only same-directory temporary files, sync, and atomic rename; existing content is verified and never silently overwritten.

## Consequences

Duplicate puts are idempotent and concurrency-safe. Corruption and truncation fail closed. A bounded local byte quota rejects new content without eviction. Chunking, databases, garbage collection, and pinning are not part of Phase 9.
