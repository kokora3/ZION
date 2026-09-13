# ADR-0057: Large-Object Chunking, Replication and Garbage Collection Deferred

## Status

Accepted.

## Decision

Phase 9 supports only complete canonical objects up to 1 MiB. Chunking, manifests, erasure coding, automatic replication, pin/unpin, eviction, garbage collection, DHT/provider records, and storage incentives are deferred.

## Consequences

The implementation stays small and auditable, but does not support large content or promise persistence once every holder removes an object.
