# ADR-0042: Outbound-Only NORMAL Nodes Are Supported

## Status

Accepted.

## Decision

Allow NORMAL nodes to run with no libp2p listen addresses and join by outbound dial, hello, and PEX.

## Consequences

Public inbound reachability is not required. Universal inbound NAT traversal is not implied.
