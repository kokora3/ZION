# v0.1 Dependency and License Inventory

The exact resolved inventories are produced in release packages as `GO-MODULES.txt` and `WEB-DEPENDENCIES.json`; `go.mod` and `apps/web/package-lock.json` are authoritative lock inputs.

## Direct Go dependencies

| Dependency | Pinned version | Observed upstream license |
|---|---:|---|
| CometBFT | 1.0.1 | Apache-2.0 |
| fxamacker/cbor | 2.9.2 | MIT |
| go-libp2p | 0.49.0 | MIT |
| go-multiaddr | 0.16.1 | MIT |
| golang.org/x/sync | 0.22.0 | BSD-3-Clause |
| golang.org/x/text | 0.40.0 | BSD-3-Clause |
| gopkg.in/yaml.v3 | 3.0.1 | MIT/Apache-2.0 notice lineage |

## Direct Web runtime dependencies

| Dependency | Pinned version | Observed package license |
|---|---:|---|
| Next.js | 16.3.5 | MIT |
| React | 19.3.0 | MIT |
| React DOM | 19.3.0 | MIT |

Development dependencies are locked in `package-lock.json` and recorded by the generated inventory. These permissive license observations are an engineering inventory, not legal advice. Transitive notices must remain available under their own terms. The release audit runs `npm audit`; `govulncheck` is run when locally available and its exact result is reported rather than inferred.

## Security-pinned transitive dependencies

The alpha release raises `github.com/pion/dtls/v3` to `v3.1.4` and
`google.golang.org/grpc` to `v1.82.1`. These are the minimum resolved versions
used to close the reachable findings reported by `govulncheck` during final
acceptance; they do not change ZION's canonical encodings or protocol IDs.
