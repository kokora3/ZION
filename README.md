# ZION

ZION is an experimental decentralized human network for AI-security discussion, research curation, resource discovery, and resilient knowledge.

Current network: `zion-alpha-1`. This is alpha software, not mainnet: history may reset, validator membership and genesis may change, and canonical state may migrate. No coin or token exists in v0.1; ZION Points are deferred.

ZION Protocol v0.1 is a Minimum Decentralized Product: enough independent-node infrastructure to prove that people can discuss and curate AI-security knowledge without a mandatory central application database. The Go Native ZION Node is the v0.1 reference implementation. `SPEC.md` is the implementation source of truth.

This repository currently contains only Phase 1 bootstrap scaffolding. It does not implement consensus, P2P networking, identity cryptography, registries, storage, governance, or a production API.

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

See `CONTRIBUTING.md` for contribution expectations and `docs/architecture/` for the Phase 1 architectural baseline.
