# Contributing to ZION

Keep changes focused and submit focused pull requests with a clear rationale. Format Go code with `gofmt` and run the relevant checks before opening a pull request:

```text
go vet ./...
go test ./...
go test -race ./...
```

Use the race detector where applicable, especially for concurrent runtime work. Keep commits focused so changes are reviewable and can be reverted independently.

Protocol changes require explicit review. `SPEC.md` is authoritative: changes that contradict it must not be silently merged. If the specification is ambiguous in a compatibility-relevant way, document the issue and seek a protocol decision before implementation.

Do not add a CLA or DCO requirement unless the repository formally adopts one.
