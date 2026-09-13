# ADR-0059: Board Posts Are Signed Off-Chain Events

## Status

Accepted.

## Decision

POST and REPLY are canonical, network-bound, Board-domain signed events stored as Phase 9 objects. Ordinary publication does not submit a chain transaction.

## Consequences

Authorship is independently verifiable without growing canonical chain state. Board authenticity is not consensus finality, and availability is best effort.
