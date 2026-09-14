# ADR-0084: Container Identity and State Are Externally Persistent

## Status

Accepted.

## Decision

All durable node data lives below the externally mounted `/var/lib/zion`; container filesystems are replaceable and read-only outside bounded temporary paths.

## Consequences

Recreation with the same volume retains PeerID, canonical state, objects, indexes, local metadata, API credential, and validator data. Sharing one mutable volume between nodes is unsupported.

