# ZION v0.1.0-alpha.1 Validation Record

Validation was performed on Windows amd64 with Go 1.27.1, CGO enabled, the
MSYS2 UCRT64 GCC toolchain, Node 22-compatible tooling, and bounded process
parallelism. Tests used temporary local directories and loopback/ephemeral
ports; no public ZION service was required.

## Protocol and system acceptance

- `TestPhase13MultiNodeProductAcceptance`: PASS (2.22s). Topology: four real
  CometBFT validator runtimes, one replaceable bootstrap, two NORMAL runtimes,
  with the second NORMAL node outbound-only. Validators converged on StateHash
  and AppHash; NORMAL nodes discovered peers and state-synced; Board posts and
  replies propagated; Research and Resource references resolved; direct P2P
  object fetch worked; bootstrap loss preserved direct connectivity; retained
  P2P key/cache/state survived restart; export/import preserved the final
  canonical hash; off-chain operations did not change StateHash.
- The export/import suite also started a fresh compatible NORMAL runtime from
  the imported snapshot, preserved height/identity/membership/governance and
  Research/Resource state, and confirmed that Board/object availability was
  not recreated by canonical import.
- `TestFourValidatorCometBFTFinalityAndFaults`: PASS (13.09s). Four and three
  validators finalized, two did not advance finality during the bounded
  observation window, and restoring quorum resumed finality.
- `TestGovernanceThroughCometBFTAndValidatorSetChanges`: PASS (11.53s).
- `TestResearchAndResourceAdmissionsThroughFourValidatorCometBFT`: PASS
  (5.28s).
- `TestFinalV01DeterminismAcrossAllCanonicalDomains`: PASS (0.02s). Independent
  allocations plus identical ordered transactions/heights produced identical
  canonical snapshot bytes, StateHash, and AppHash; differing local object
  availability did not affect the result.
- Existing Phase 2-12 golden fixtures were not modified. Full package tests
  passed their canonical, atomicity, corruption, restart, rebuild, quota,
  API-security, Board search/hide/offline-sync, and state-sync checks.
- Occupied API TCP, libp2p QUIC/UDP, and CometBFT P2P/TCP ports all produced a
  clean startup failure without canonical-state mutation. The CometBFT case
  exposed and fixed a failed-start cleanup leak that had left database handles
  open on Windows; the final tests confirmed those handles are released.

## Toolchain validation

| Check | Result |
|---|---|
| `gofmt` / `gofmt -l` | PASS / no unformatted Go files |
| `go mod tidy` | PASS |
| `go vet ./...` | PASS |
| `go test -p=1 ./...` | PASS |
| `go test -race -p=1 ./...` | PASS; no data races |
| `go build ./cmd/zion-node` | PASS |
| `go build ./cmd/zionctl` | PASS |
| `npm ci` | PASS; 453 packages installed, 0 vulnerabilities reported |
| `npm run lint` | PASS |
| `npm run typecheck` | PASS |
| `npm test` | PASS; 3 files, 20 tests |
| `npm run build` | PASS |
| `npm run e2e` | PASS; 8/8 Playwright product tests |
| `TestOptionalSoak` with `ZION_SOAK_SECONDS=60` | PASS (60.11s) |

Go commands emitted non-fatal Windows module stat-cache access warnings for
the main dirty working-tree pseudo-version during some binary builds. All
listed commands completed with exit code zero; the warning is not an
application failure.

## Fuzz smoke

Each target ran with one worker and a two-second requested fuzz duration. All
24 targets passed without a panic:

| Package / target | Executions |
|---|---:|
| protocol / `FuzzParseObjectID` | 50,046 |
| protocol / `FuzzDecodeUnsignedObjectCore` | 31,020 |
| identity / `FuzzParseIdentityID` | 36,362 |
| identity / `FuzzParseKeyID` | 25,462 |
| identity / `FuzzPublicKeyValidate` | 27,188 |
| identity / `FuzzSignatureValidate` | 34,087 |
| chain / `FuzzParseTxID` | 8,290 |
| governance / `FuzzParseProposalID` | 45,504 |
| consensus / `FuzzDecodeTransaction` | 61 |
| p2p / `FuzzDecodeHello` | 53 |
| p2p / `FuzzDecodePEX` | 137 |
| node / `FuzzDecodeTxRelayMessage` | 1,154 |
| node / `FuzzDecodeStateSyncMessage` | 1,443 |
| objects / `FuzzDecodeObjectGetRequest` | 8,264 |
| objects / `FuzzDecodeObjectGetResponse` | 622 |
| board / `FuzzDecodeBoardContent` | 4,094 |
| board / `FuzzDecodeBoardEvent` | 5 |
| board / `FuzzDecodeBoardAnnounce` | 8 |
| board / `FuzzDecodeBoardSyncRequest` | 1,499 |
| board / `FuzzDecodeBoardSyncResponse` | 3 |
| research / `FuzzParseResearchID` | 30,173 |
| research / `FuzzDecodeResearchEntry` | 28,794 |
| resources / `FuzzParseResourceID` | 31,684 |
| resources / `FuzzDecodeResourceEntry` | 22,862 |

## Dependency and vulnerability disposition

The first final `govulncheck` found three reachable findings in transitive
dependencies: GO-2026-6165 in `github.com/pion/dtls/v3@v3.1.2`, and
GO-2026-6061 plus GO-2026-4762 in `google.golang.org/grpc@v1.70.0`. The
resolved graph was raised to DTLS `v3.1.4` and gRPC `v1.82.1`. Focused and full
regressions passed. The post-fix scan reported: **0 vulnerabilities affecting
called code**. `npm audit` reported 0 vulnerabilities. Direct dependency and
license observations are in `dependencies-v0.1.md`; generated binary and Web
dependency inventories accompany the release archives.

## Release artifacts

The Windows artifact was executed locally and reported the injected version,
commit, and build date. Linux amd64 was cross-compiled with CGO disabled and
its Go build/module metadata was inspected; it was not runtime-executed on the
Windows validation host.

```text
c8a058a7819d3259e59fb37b2d09d919ab1a91ebb8168bf335a8c4e8d3fda657  zion-v0.1.0-alpha.1-linux-amd64.tar.gz
3b415fc180d18c641b1e67d303139dcd02296c1d75a9c819e7916b755ceaaefb  zion-v0.1.0-alpha.1-windows-amd64.zip
```

`scripts/verify-release.ps1` verified both archive checksums and the release
secret-file-name boundary. The packages are local unsigned alpha artifacts;
no upload or public deployment was performed.
