# ZION

ZION is an experimental decentralized human network for AI-security discussion, research curation, resource discovery, and resilient knowledge.

Current network: `zion-alpha-1`. This is alpha software, not mainnet: history may reset, validator membership and genesis may change, and canonical state may migrate. No coin or token exists in v0.1; ZION Points are deferred.

ZION Protocol v0.1 is a Minimum Decentralized Product: enough independent-node infrastructure to prove that people can discuss and curate AI-security knowledge without a mandatory central application database. The Go Native ZION Node is the v0.1 reference implementation. `SPEC.md` is the implementation source of truth.

This repository implements Phases 1–11: canonical protocol and identity primitives, deterministic chain state, CometBFT finality, minimal governance, authenticated general libp2p discovery, a unified durable node runtime and local API, a small content-addressed object store with bounded direct peer retrieval, a signed off-chain community Board, and governance-admitted canonical Research and Resource registries. ZION retains its native transaction, identity, membership, state, and hash semantics; CometBFT v1.0.1 supplies permissioned validator ordering and finality.

Phase 11 adds immutable, content-derived ResearchID and ResourceID records after canonical governance execution. Registry metadata and canonical references enter StateHash/AppHash, while object availability and local search indexes do not. Board links to registry IDs remain signed off-chain community statements. Graph inference, large-file features, replication/GC, automatic repository execution/update, and automated software migration remain deferred.

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
