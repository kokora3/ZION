# Network Bootstrap Architecture

The intended v0.1 discovery order is:

1. persisted peer cache;
2. configured or DNS bootstrap seeds;
3. static fallback bootstrap addresses;
4. a manually supplied peer address.

The peer cache is empty on a true first boot. Bootstrap nodes are discovery helpers, not a source of truth, and bootstrap addresses must not become immutable chain state. They must remain replaceable.

A bootstrap endpoint should pair a DNS or IP location with a cryptographic peer identity. DNS/IP location tells a node where to connect; it is not the same as peer identity and is insufficient on its own.

NORMAL nodes behind NAT may participate by initiating outbound connections. Inbound reachability is not required for NORMAL nodes. VALIDATOR and BOOTSTRAP nodes should normally be publicly reachable.

Hole punching, AutoNAT, and relay support are intentionally deferred beyond v0.1. No discovery, transport, caching, DNS, or bootstrap mechanism is implemented in Phase 1.
