# ADR-0069: Object Availability Is Not a Consensus Precondition

## Status

Accepted.

## Decision

Apply validates ObjectID syntax but never consults local or network object availability.

## Consequences

Validators with different local object stores derive identical ResearchIDs, ResourceIDs, StateHashes, and AppHashes.
