# ADR-0060: post_id Is the Signed Board Event ObjectID

## Status

Accepted.

## Decision

`post_id` is the unchanged Phase 2 ObjectID of the complete canonical signed-event object. Content has its own separate ObjectID.

## Consequences

The identifier commits to event fields and signature, cannot be self-referential, and requires no new hash namespace.
