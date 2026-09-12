# ADR-0028: Validator Consensus Identity Separate from Member Identity

## Status
Accepted.

## Context
Member authorship, consensus voting, and future P2P peering have different security roles.

## Decision
Keep CometBFT validator keys separate from ZION `IdentityID` keys and future P2P `PeerID` keys. Never place validator private keys in canonical state or fixtures.

## Consequences
Validator operation does not grant application membership or authorization. Key compromise and rotation can be handled within the correct domain in later phases.
