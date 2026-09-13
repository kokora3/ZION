# ADR-0051: State Sync Is Hash-Verified Alpha Snapshot Synchronization

## Status

Accepted.

## Decision

Verify authenticated source, network, GenesisID, canonical bytes, height, and StateHash while making no BFT light-client claim.

## Consequences

Same-height conflicts fail; checkpoint/light-client proof remains future work.
