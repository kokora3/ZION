# ADR-0071: Registry Entries Are Immutable in v0.1

## Status

Accepted.

## Decision

Do not update or overwrite admitted entries. A body change creates a new typed ID; duplicate admission is rejected.

## Consequences

Identity and provenance are stable and auditable. Revision and supersession workflows are deferred.
