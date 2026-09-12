# ADR-0034: Integer Strict-Two-Thirds Approval

## Status

Accepted.

## Context

Floating-point threshold calculations can differ across implementations, and abstention should not be identical to a NO vote.

## Decision

Require at least three unique participants. After that, approve exactly when YES * 3 > (YES + NO) * 2. ABSTAIN counts toward participation but not the approval denominator.

## Consequences

The exact 2/3 boundary rejects, all math is deterministic integer math, and a three-voter YES, YES, ABSTAIN outcome approves.
