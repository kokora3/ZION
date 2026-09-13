# ADR-0055: Object Availability Is Off-Chain Operational State

## Status

Accepted.

## Decision

Object files, store accounting, quota, paths, timestamps, peer availability, and fetch outcomes are local operational state and never inputs to canonical transactions, snapshots, `StateHash`, or `AppHash`.

## Consequences

Nodes may hold different object sets while agreeing on the chain. Store failure cannot grant authority or rewrite canonical state, and changing storage policy requires no governance action or genesis reset.
