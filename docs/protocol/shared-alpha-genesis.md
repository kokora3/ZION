# Shared `zion-alpha-1` genesis

## Frozen identity

G1 freezes the first shared Internet-alpha network instance:

```text
NetworkID: zion-alpha-1
GenesisID: 72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6
CometBFT chain ID: zion-alpha-1-72ef0c7816d6
Genesis schema: ZION application genesis v1 in a CometBFT v1.0.1 genesis document
Genesis time: 2026-09-14T00:00:00Z
```

The authoritative files are `configs/alpha-1/genesis.json`, `configs/alpha-1/genesis-id.txt`, and `configs/alpha-1/validators-public.json`.

`internal/consensus.NewGenesis` is the only GenesisID derivation path. It hashes the canonical Phase 4 empty-state snapshot, sorts validators by raw public-key bytes, hashes that public validator set with canonical CBOR, builds the application-genesis JSON containing schema version, NetworkID, StateHash, and validator-set hash, then SHA-256 hashes those exact JSON bytes. No shell, deployment, or documentation code reimplements the digest.

The CometBFT genesis timestamp is explicit and frozen so the complete public genesis file regenerates byte-for-byte. It is operational CometBFT metadata and is not a field in the ZION application genesis, so it does not participate in GenesisID. CI nevertheless rejects timestamp or byte changes through the exact genesis-file guard.

## Validator set and identity boundaries

The genesis contains four unique Ed25519 consensus public keys with voting power 1. Ordering is ascending raw public-key byte order. The validator key is not a ZION member IdentityID, libp2p PeerID, hostname, IP address, bootstrap role, or local role toggle. The initial application state is the existing empty Phase 4 state; no identity, membership, or governance record was invented and no undocumented member-to-validator coupling was added.

The four private validator keys were generated with CometBFT's cryptographically secure Ed25519 generator. They remain only under the operator-controlled `.local/alpha-1/validators/validator-N/` directories and are ignored by Git and Docker. Each host later receives only its own `priv_validator_key.json` and signing state. Public genesis data cannot recover a lost private key.

Back up each validator directory separately using encrypted offline storage and restrict access to its operator. POSIX-capable systems should retain directory mode 0700 and file mode 0600. Windows mode bits do not fully describe NTFS ACL protection; the operator must verify the account ACL and encrypted backup controls. The generator refuses an existing output directory or manifest and never overwrites a persistent validator identity.

Private-key distribution and validator hosting are outside G1. Conceptually:

```text
validator-1 private key -> validator host 1 only
validator-2 private key -> validator host 2 only
validator-3 private key -> validator host 3 only
validator-4 private key -> validator host 4 only
```

## Offline generation and verification

Initial operator generation, performed once in the trusted operator workspace:

```text
go run ./cmd/zionctl genesis keys --out-dir .local/alpha-1/validators --manifest configs/alpha-1/validators-public.json --genesis-time 2026-09-14T00:00:00Z
go run ./cmd/zionctl genesis build --manifest configs/alpha-1/validators-public.json --out configs/alpha-1/genesis.json --id-out configs/alpha-1/genesis-id.txt
```

Those commands refuse to overwrite files. Routine verification is read-only and has no network I/O:

```text
go run ./cmd/zionctl genesis verify --manifest configs/alpha-1/validators-public.json --genesis configs/alpha-1/genesis.json --genesis-id configs/alpha-1/genesis-id.txt
```

Add `--json` for stable machine-readable CI output. Verification rebuilds the expected document through `internal/consensus`, requires exact genesis bytes, loads it through the production `consensus.LoadGenesis` validator, and checks `genesis-id.txt` exactly.

For the explicit four-real-key local acceptance test, keep the keys outside CI and run:

```text
ZION_ALPHA_VALIDATOR_DIR=.local/alpha-1/validators go test ./internal/consensus -run TestSharedGenesisWithOperatorKeys -count=1
```

In PowerShell, set `$env:ZION_ALPHA_VALIDATOR_DIR='.local/alpha-1/validators'` before the `go test` command. The harness copies keys into temporary CometBFT roots; it does not upload or commit them.

## Shared versus local/test networks

The digest `4cce10c9eb93aa5baff6ec94b13ff27662464668764a39efed6d59373a487d55` remains D1 local-only. Phase 5 deterministic validator keys and their GenesisIDs remain test-only. CI proves neither set equals or overlaps the shared alpha set. Nodes with those local/test fingerprints are incompatible even though the NetworkID string may also be `zion-alpha-1`.

DNS names, IP addresses, DigitalOcean regions, bootstrap PeerIDs, P2P addresses, node roles, and private keys are not bound into GenesisID. They are replaceable operational state. Once the three public files are distributed, verification needs no GitHub, DigitalOcean, Cloudflare, DNS, or other hosted dependency.

## D2A boundary and freeze policy

D2A reads `configs/alpha-1/genesis-id.txt` and distributes the matching public genesis. The first DigitalOcean NORMAL + BOOTSTRAP node is not a validator and receives no validator private key. It needs only the frozen public network identity, its independent P2P identity, and its bootstrap configuration.

Changing the shared `zion-alpha-1` genesis creates a different network instance. Normal fixes and packaging changes must not regenerate it. If a critical pre-deployment defect requires a reset, record the reason in a new ADR/change log, intentionally regenerate all dependent artifacts, update GenesisID, and publish an explicit reset/migration boundary. A new genesis must never masquerade under the old GenesisID.
