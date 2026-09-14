# ADR-0074: ZION Web Does Not Reimplement Canonical Protocol Logic

## Status

Accepted.

## Decision

Do not implement canonical CBOR, identifiers, hashes, signatures, governance thresholds, chain Apply, consensus, or P2P in TypeScript. Use typed JSON projections and normal node submission paths.

## Consequences

The Go node remains authoritative and browser upgrades cannot fork consensus semantics.
