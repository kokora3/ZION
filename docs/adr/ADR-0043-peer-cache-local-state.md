# ADR-0043: Peer Cache Is Local Non-Consensus State

## Status

Accepted.

## Decision

Persist bounded, versioned JSON records. Only peers that complete transport authentication and hello are eligible as trusted reconnect candidates; rejected candidates may be retained solely as bounded failure/backoff records. Use local time only for cache ordering and dial backoff.

## Consequences

Cache migration can evolve independently. Corruption or compromise cannot authorize canonical transitions or change StateHash.
