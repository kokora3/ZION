# zion-alpha-1 genesis

The authoritative shared Internet-alpha genesis is `genesis.json`.

```text
NetworkID: zion-alpha-1
GenesisID: 72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6
Validators: 4 distinct Ed25519 consensus keys, power 1 each
```

`validators-public.json` is the public, explicit generator input. `genesis-id.txt` is the single machine-readable digest for D2A and other consumers. Do not edit any of these files manually. Verify them offline from the repository root:

```text
go run ./cmd/zionctl genesis verify --manifest configs/alpha-1/validators-public.json --genesis configs/alpha-1/genesis.json --genesis-id configs/alpha-1/genesis-id.txt
```

Files ending in `.example.yaml` are shared-alpha role templates and carry the frozen GenesisID. `normal.yaml`, `bootstrap.yaml`, and `validator.yaml.example` retain D1's explicitly local-only GenesisID for isolated packaging tests; they must not be used to join the shared Internet alpha.

Changing the shared genesis creates a different network instance and requires an explicit reset/migration decision and ADR. Private validator files never belong in this directory, Git, Docker build contexts, release archives, or CI artifacts.
