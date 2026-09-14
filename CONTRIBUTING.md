# Contributing to ZION

Keep changes focused and submit focused pull requests with a clear rationale. Format Go code with `gofmt` and run the relevant checks before opening a pull request:

```text
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/zion-node
go build ./cmd/zionctl
```

Use the race detector where applicable, especially for concurrent runtime work. Keep commits focused so changes are reviewable and can be reverted independently.

Protocol changes require explicit review. `SPEC.md` is authoritative: changes that contradict it must not be silently merged. If the specification is ambiguous in a compatibility-relevant way, document the issue and seek a protocol decision before implementation.

Do not add a CLA or DCO requirement unless the repository formally adopts one.

Frontend changes must also pass `npm ci`, `npm run lint`, `npm run typecheck`, `npm run test`, `npm run build`, and the local-runtime Playwright suite in `apps/web`. Tests must not require public Internet services.

Never update a stored golden fixture as a mechanical way to make a test pass. Explain the protocol decision and compatibility impact first. Deterministic fixture keys must retain the warning **TEST ONLY — PUBLIC FIXTURE — NOT SECRET — NEVER USE IN PRODUCTION** and must never be reused operationally.

New parsers and network frames need explicit size/count bounds, malformed-input tests, and an owned decoder fuzz target where raw decoding exists. Canonical code must remain independent of wall clock, randomness, filesystem/network state, logs, metrics, caches, and UI state. Run `scripts/fuzz-smoke.ps1` for the bounded full decoder campaign when changing these boundaries.
