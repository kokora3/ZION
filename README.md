# ZION

ZION is an experimental decentralized human network for AI-security discussion, research curation, resource discovery, and resilient knowledge.

Current network: `zion-alpha-1`. This is alpha software, not mainnet: history may reset, validator membership and genesis may change, and canonical state may migrate. No coin or token exists in v0.1; ZION Points are deferred.

ZION Protocol v0.1 is a Minimum Decentralized Product: enough independent-node infrastructure to prove that people can discuss and curate AI-security knowledge without a mandatory central application database. The Go Native ZION Node is the v0.1 reference implementation. `SPEC.md` is the implementation source of truth.

This repository implements Phases 1–9: canonical protocol and identity primitives, deterministic chain state, CometBFT finality, minimal governance, authenticated general libp2p discovery, a unified durable node runtime and local API, and a small content-addressed object store with bounded direct peer retrieval. ZION retains its native transaction, identity, membership, state, and hash semantics; CometBFT v1.0.1 supplies permissioned validator ordering and finality.

Phase 9 stores complete canonical objects up to 1 MiB, reuses the frozen Phase 2 ObjectID, verifies content on every read and remote fetch, and keeps availability strictly outside StateHash/AppHash. It is a best-effort local/off-chain facility—not permanent distributed storage, a DHT, confidentiality layer, large-file system, or registry. Board, Research Registry, Resource Registry, replication/GC, and automated software migration remain deferred.

## Development

Requires Go 1.27 or newer.

```text
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/zion-node
go build ./cmd/zionctl
```

```text
zion-node --version
zionctl --version
```

See `CONTRIBUTING.md` for contribution expectations and `docs/architecture/` for protocol architecture notes.
