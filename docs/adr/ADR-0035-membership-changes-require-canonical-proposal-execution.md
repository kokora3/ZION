# ADR-0035: Membership Changes Require Canonical Proposal Execution

## Status

Accepted.

## Context

Identity possession does not grant membership, and local configuration cannot safely decide runtime membership transitions.

## Decision

Membership changes after bootstrap require an APPROVED governance proposal and a separate execution transaction. The proposal contains the expected and requested status; execution revalidates the expected status and applies the transition once.

## Consequences

Direct MembershipChange stays unsupported. Stale and repeated execution fail atomically, while the one-time deterministic genesis bootstrap creates the initial ACTIVE electorate without a permanent superuser.
