# ADR-0026: CometBFT as the ZION v0.1 Reference Consensus Engine

## Status
Accepted.

## Context
ZION needs permissioned BFT ordering and finality without creating a novel consensus algorithm.

## Decision
Pin CometBFT v1.0.1 (Apache-2.0) as the reference engine. Do not introduce Cosmos SDK or implement BFT voting rules in ZION.

## Consequences
CometBFT owns rounds, votes, proposer selection, evidence, and validator consensus transport. ZION owns all application protocol semantics.
