# ADR-0065: Local Board Hiding Does Not Mean Global Deletion

## Status

Accepted.

## Decision

Hide/unhide changes only a node's derived local visibility metadata. It does not delete immutable objects or send a global moderation command.

## Consequences

Operators control their local views, while public content may remain on independent peers. ZION makes no guaranteed-deletion claim.
