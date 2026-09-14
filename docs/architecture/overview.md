# Architecture Baseline

The frozen v0.1 MDP preserves the package and authority boundaries in `SPEC.md`.

```text
protocol (runtime-independent identifiers and future compatibility primitives)
  ↑
chain, consensus, p2p, identity, membership, governance, board, research,
resources, objects, index, api, config, resource (separate lower-level domains)
  ↑
node (lifecycle, persistence, relay, sync, local projections)
  ↑
zion-node; local API clients such as zionctl and ZION Web
```

`internal/node` may depend on lower-level packages. Lower-level protocol packages must not depend on `internal/node`. The Go dependency graph must remain acyclic.

Canonical authority flows only through ordered transactions and the chain state machine. CometBFT establishes ordering/finality; libp2p, persistence, API, objects, Board, indexes, Web, logs, metrics, and release tooling cannot independently authorize canonical changes.
