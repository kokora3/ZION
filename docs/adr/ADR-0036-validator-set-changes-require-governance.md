# ADR-0036: Validator-Set Changes Require Governance Authorization

## Status

Accepted.

## Context

ADR-0031 deferred runtime validator changes until governance authorization existed.

## Decision

Only execution of an APPROVED validator-set proposal may emit an ABCI validator update. The payload separates operator IdentityID from consensus public key, fixes power at 1, guards the expected set hash, requires an ACTIVE operator, and prevents removal below three validators.

## Consequences

CometBFT applies an update returned at height H at H+2 and retains responsibility for BFT quorum. Local configuration and direct transactions cannot change the established set.
