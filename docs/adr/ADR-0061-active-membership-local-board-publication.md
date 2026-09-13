# ADR-0061: ACTIVE Membership Is Required for Local Board Publication

## Status

Accepted.

## Decision

Local publication requires current ACTIVE membership, the identity's current active key, a valid Board-domain signature, and valid local content. PENDING, SUSPENDED, REVOKED, unknown, and retired-key authors are rejected.

## Consequences

Chain state gates local creation without putting Board payloads on-chain. Remote historical events are retained only with separate honest authorization labels.
