# ADR-0088: Public Bootstrap Deployment Reuses D1 with Explicit Advertised Addresses

## Status

Accepted for D2A.

## Decision

Run the first public node as NORMAL + BOOTSTRAP using the unchanged D1 `zion-node` image and canonical root Compose model. Add only `deploy/public-host/compose.override.yml` for generic host bind mounts, a pinned image reference, UDP publication, bounded logs, and a public libp2p advertised address.

Separate libp2p bind addresses from explicit advertised addresses. The node binds container UDP 42000 but advertises the operator-supplied public DNS/IP QUIC multiaddr; runtime status appends the persisted cryptographic PeerID to produce a copyable bootstrap multiaddr. Explicit IP advertisements must be publicly routable. DNS/IP location, PeerID, and frozen GenesisID remain independent.

Publish neither API/metrics nor Web. Mount no validator material and enable no consensus runtime. Provider resource creation, DNS/firewall mutation, and provider mappings remain D2B work.

## Consequences

Container recreation and image updates retain PeerID and state through `/var/lib/zion`. A provider move changes only the advertised location and does not change chain identity. Operators must arrange the host, stable address, firewall, Docker installation, and pinned-image distribution outside ZION runtime behavior.
