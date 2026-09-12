# ADR-0029: Four-Validator Equal-Power Alpha Consensus

## Status
Accepted.

## Context
The alpha network needs a small deterministic permissioned validator model without economics.

## Decision
Use exactly four distinct genesis validators, each with voting power one. Reject duplicate keys, invalid definitions, and unequal power.

## Consequences
Three validators satisfy CometBFT's greater-than-two-thirds threshold; one may be offline. Two validators cannot finalize. Power is unrelated to points, money, age, reputation, or capacity.
