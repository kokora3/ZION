# ADR-0025: Membership Governance Authorization Deferred

## Status
Accepted.

## Context
The specification names `MembershipChange`, but Phase 4 has no approved governance authorization or voting mechanism.

## Decision
Reserve the transaction name but deterministically reject `MembershipChange`. Identity creation initializes `PENDING`; no local configuration grants authority to change membership.

## Consequences
Phase 4 preserves the membership state boundary without inventing governance. A later specification decision must define authorization before membership changes can execute.
