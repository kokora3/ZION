# ZION

ZION is an experimental decentralized human network for AI-security discussion, research curation, resource discovery, and resilient knowledge.

Current network: `zion-alpha-1`. This is alpha software, not mainnet: history may reset, validator membership and genesis may change, and canonical state may migrate. No coin or token exists in v0.1; ZION Points are deferred.

ZION Protocol v0.1 is a Minimum Decentralized Product: enough independent-node infrastructure to prove that people can discuss and curate AI-security knowledge without a mandatory central application database. The Go Native ZION Node is the v0.1 reference implementation. `SPEC.md` is the implementation source of truth.

This repository includes the Phase 1 architecture, Phase 2 canonical protocol primitives, Phase 3 identity and membership primitives, the Phase 4 deterministic chain state machine, and the Phase 5 CometBFT consensus adapter. ZION retains its native transaction, identity, membership, state, and hash semantics; CometBFT v1.0.1 supplies permissioned validator ordering and finality. The Phase 5 harness proves four-validator finality, ordered execution, one-validator tolerance, quorum-loss safety, and recovery. General ZION P2P, durable application persistence, governance voting, registries, and runtime validator-set changes remain deferred.

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
