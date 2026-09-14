# ADR-0087: zion-alpha-1 Shared Genesis Is a Frozen Deterministic Network Identity

## Status

Accepted for G1.

## Context

D1 had a clearly local-only GenesisID and shared-alpha templates still had placeholders. Neither local nor Phase 5 test validator identities are suitable authority for an Internet alpha. D2A therefore had no unambiguous public chain identity to deploy.

## Decision

Freeze `configs/alpha-1/genesis.json` as the authoritative shared `zion-alpha-1` genesis with GenesisID `72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6`.

Derive it only through `internal/consensus.NewGenesis`, which binds the NetworkID, existing empty application StateHash, and the canonical public validator-set hash. Use four unique Ed25519 consensus keys at equal power 1, sorted by raw public-key bytes. Keep validator consensus identity separate from member IdentityID, libp2p PeerID, bootstrap role, and deployment location.

Freeze the CometBFT timestamp at `2026-09-14T00:00:00Z` for whole-file reproducibility. It remains outside the ZION GenesisID derivation. Commit only public validator material; keep the cryptographically random private keys in the gitignored operator workspace and out of Docker, releases, and ordinary CI.

## Consequences

All shared-alpha nodes can verify one NetworkID, GenesisID, initial application state, and validator set locally. The local-only D1 and deterministic test networks remain distinct and incompatible. DNS, IP, bootstrap PeerID, cloud provider, and validator host mapping can change without changing chain identity.

Any edit that changes the shared genesis creates a different network instance. Before external alpha launch, an intentional replacement requires a documented reason and updated ADR/change record. After launch it additionally requires an explicit reset/migration decision. Silent regeneration is forbidden.
