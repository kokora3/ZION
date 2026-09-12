# ADR-0044: ZION P2P Hello Binds Network, Genesis, and Version

## Status

Accepted.

## Decision

Use canonical CBOR on `/zion/hello/0.1.0`, require the reported PeerID to match transport authentication, bind NetworkID and the Phase 5 GenesisID fingerprint, and negotiate an explicit typed version intersection.

## Consequences

Wrong network, same-name wrong genesis, no common version, malformed roles, malformed addresses, and oversized or non-canonical messages never become usable ZION sessions.
