# Architecture Baseline

Phase 1 preserves the architectural boundaries in `SPEC.md` without implementing protocol behavior.

```text
protocol (runtime-independent identifiers and future compatibility primitives)
  ↑
chain, consensus, p2p, identity, membership, governance, board, research,
resources, objects, index, api, config, resource (separate lower-level domains)
  ↑
node (future lifecycle and runtime orchestration)
  ↑
zion-node; local API clients such as zionctl and ZION Web
```

`internal/node` may depend on lower-level packages. Lower-level protocol packages must not depend on `internal/node`. The Go dependency graph must remain acyclic.

No canonical encoding, chain state machine, identity, consensus, P2P stack, storage, API, or UI behavior exists in this phase.
