# ADR-0023: Atomic Failed Transactions

## Status
Accepted.

## Context
A rejected transaction that partially changes identity, key, sequence, or membership state would make replay unsafe and validator results divergent.

## Decision
Apply transactions copy-on-write and publish new state only after every deterministic validation succeeds. Rejection returns the unchanged canonical state and a deterministic receipt containing its pre-transaction `StateHash`.

## Consequences
Callers may safely continue from rejected transactions, and tests can prove atomicity by comparing canonical bytes and StateHash before and after each failure path.
