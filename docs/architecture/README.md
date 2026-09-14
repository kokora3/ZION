# ZION v0.1 Architecture Index

`SPEC.md` is authoritative. The documents below explain the implemented v0.1 boundaries; ADRs record decisions.

- [Overview](overview.md) — package layering and authority boundaries.
- [Identity](identity.md) — IdentityID, KeyID, proof of possession, and rotation.
- [Consensus](consensus.md) — CometBFT ordering/finality and AppHash mapping.
- [State machine](state-machine.md) — deterministic Apply, Snapshot, and StateHash.
- [Governance](governance.md) — ACTIVE-member voting and canonical execution.
- [General P2P](p2p.md) — libp2p discovery, hello, PEX, and PeerID.
- [Node runtime](node-runtime.md) — lifecycle, durable state, relay, sync, and local API.
- [Object store](object-store.md) — immutable small objects and bounded retrieval.
- [Board](board.md) — signed off-chain posts, replies, sync, and local search.
- [Registries](registries.md) — governance-admitted Research and Resource metadata.
- [Web client](web-client.md) — replaceable browser UX over `/v1`.
- [State recovery](state-recovery.md) — explicit export/import envelope and safety.
- [Observability](observability.md) — health, metrics, structured logs, and cardinality.

No layer delegates canonical authority to the Web client, API, bootstrap peers, DNS, PEX, object availability, Board indexes, registry search indexes, metrics, logs, or release infrastructure.
