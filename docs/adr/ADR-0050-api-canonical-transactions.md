# ADR-0050: API Submits Canonical Signed Transactions

## Status

Accepted.

## Decision

Accept base64-wrapped canonical bytes and route them through the existing decoder, relay, and consensus path.

## Consequences

The API cannot create a second transaction format or bypass governance.
