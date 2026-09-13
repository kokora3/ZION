# ADR-0072: Registry Search Indexes Are Local Rebuildable Derived State

## Status

Accepted.

## Decision

Build bounded list/get/search projections from canonical registry state and persist them in a versioned local JSON index.

## Consequences

Missing or corrupt index files can be replaced without data loss or StateHash changes. Index ordering and availability are not consensus inputs.
