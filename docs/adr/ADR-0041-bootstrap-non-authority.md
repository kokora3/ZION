# ADR-0041: Bootstrap Peers Are Discovery Infrastructure, Not Authority

## Status

Accepted.

## Decision

Treat configured, DNS, and fallback bootstrap peers as replaceable introducers whose addresses include an expected cryptographic PeerID.

## Consequences

Bootstrap configuration is local non-consensus state. Bootstrap claims and PEX hints grant no membership, governance, validator, or canonical-state authority.
