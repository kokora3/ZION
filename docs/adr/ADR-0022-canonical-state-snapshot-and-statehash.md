# ADR-0022: Canonical State Snapshot and StateHash

## Status
Accepted.

## Context
Go maps, pointers, allocation patterns, and slice capacity cannot be allowed to change consensus state bytes.

## Decision
Convert in-memory state to a validated canonical snapshot with identities, public keys, and memberships in explicit identifier order. Deep-copy byte slices and hash the canonical snapshot CBOR with SHA-256. Render the result as `zion:state:sha256:<lowercase-hex>`.

## Consequences
Logical state, rather than process representation, determines `StateHash`. Invalid or inconsistent in-memory state cannot silently produce a canonical snapshot.
