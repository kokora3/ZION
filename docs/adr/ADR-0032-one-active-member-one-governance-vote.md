# ADR-0032: One ACTIVE Member, One Governance Vote

## Status

Accepted.

## Context

Governance needs a voting unit without introducing token, stake, validator-power, reputation, identity-age, or hardware weighting.

## Decision

Each currently ACTIVE member IdentityID has one vote on proposals whose electorate snapshot contains that identity. Votes are recorded by IdentityID, so key rotation cannot create another vote.

## Consequences

Governance power is equal per eligible member and distinct from CometBFT voting power. PENDING, SUSPENDED, and REVOKED identities cannot vote.
