# ADR-0047: Durable Application Snapshots Are Separate from CometBFT Storage

## Status

Accepted.

## Decision

Persist exact canonical snapshot bytes in a versioned, network/genesis/hash-bound local envelope.

## Consequences

CometBFT databases are not the ZION application storage format; corrupt state fails closed without reset.
