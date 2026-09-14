# ADR-0079: Canonical State Export and Import Is Explicit and Secret-Free

## Status

Accepted.

## Decision

Use a bounded version-1 JSON export envelope containing the existing canonical Snapshot CBOR bytes, NetworkID, GenesisID, state schema, accepted height, and StateHash. Export, inspection, and import are explicit `zion-node state` operations. Import recomputes all integrity values, does not overwrite existing state, and is limited to a stopped, fresh NORMAL-node target. The existing deterministic V2-to-V3 migration is the only older-schema migration supported.

## Consequences

Canonical state can be inspected and recovered without inventing another state representation or exporting P2P, validator, member, API, object, Board, cache, or index secrets/state. Arbitrary validator database recovery remains an operator procedure, not this import command.
