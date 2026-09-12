# ADR-0033: Governance Electorate Snapshot

## Status

Accepted.

## Context

Membership may change while a proposal is open, which could make eligibility nondeterministic if validators used their current set as the proposal denominator.

## Decision

Snapshot the sorted IdentityID set of ACTIVE members when a proposal opens. A voter must be in that snapshot and remain currently ACTIVE when voting.

## Consequences

Later activation does not add a voter to an open proposal. Suspension or revocation prevents a snapshotted member from casting a new vote, while preserving the original electorate record.
