# Deterministic Chain State Machine

The Phase 4 chain package is ZION's compact, deterministic trusted-state layer. Consensus and the state machine are separate: a future consensus implementation may decide one final transaction order, while this package only executes that order. It contains no BFT protocol, block model, mempool, networking, persistence, or governance voting.

For a valid transaction, every validator evaluates the same transition:

```text
Apply(S, T) -> S'
```

The only inputs are the previous canonical state `S` and transaction `T`. Local time, randomness, filesystem ordering, process-specific map iteration, and external network or API state are not inputs.

```text
Ordered Transactions
        |
        v
Deterministic State Machine
        |
        +-- Identity State
        +-- Membership State
        |
        v
Canonical Snapshot
        |
        v
StateHash
```

## Transactions and TxID

A transaction is a schema-versioned, network-bound canonical CBOR envelope. Its selected payload carries the Phase 3 cryptographic proof. `IdentityCreate` requires an identity genesis proof; `KeyRotation` requires both old-key authorization and new-key proof of possession. Payloads that are missing, mismatched, or ambiguous are rejected.

`TxID` is SHA-256 over the complete canonical transaction bytes and renders as `zion:tx:sha256:<lowercase-hex>`. The transaction structure has no `TxID` field, so the digest cannot recursively include itself. Parsing is strict about namespace, algorithm, lowercase hexadecimal, digest length, and component count.

Transactions are applied exactly in the supplied order. The state machine never sorts or batches them. Consequently, `IdentityCreate` followed by `KeyRotation` produces a rotated identity, while `KeyRotation` followed by `IdentityCreate` first rejects the unknown identity and ends with the unrotated identity.

## Canonical State, Snapshot, and StateHash

In-memory `State` uses maps for lookup. The canonical `Snapshot` converts all consensus maps to sorted slices: identities sort by `IdentityID`, keys sort by `KeyID`, and memberships sort by `IdentityID`. It validates that map keys match their content-derived IDs, key/status sets are complete, public keys are valid, and every identity has exactly one membership record. Snapshot construction deep-copies protocol byte slices.

These rules make canonical state bytes independent of Go map insertion order, pointer identity, independent record allocation, backing-array capacity, and caller-owned transaction storage. Canonical state contains public keys and public proof-derived state only; private keys and Ed25519 seeds are excluded.

`StateHash` is SHA-256 over the canonical snapshot CBOR and renders as `zion:state:sha256:<lowercase-hex>`. The empty genesis state is deterministic for a given network, schema, and protocol version. Stored Phase 4 fixtures freeze transaction bytes, TxIDs, snapshot bytes, SHA-256 digests, and StateHashes for cross-language compatibility.

## Atomic failure behavior

`Apply` clones state before attempting a transition and publishes the clone only after all validation succeeds. A rejected transaction returns the original canonical state. Its deterministic receipt contains the transaction ID when canonical encoding succeeded, transaction type, result code, and unchanged pre-transaction `StateHash`; it contains no clock value, random value, address, stack trace, or process identifier.

Atomic rejection covers duplicate creation, wrong networks, unsupported schemas and transaction types, unknown identities, wrong sequence, non-active or retired old keys, malformed proofs, incorrect new-key proof signers, and oversized transactions.

## IdentityCreate

`IdentityCreate` verifies the real Phase 3 identity genesis proof, including the content-derived `IdentityID`, initial `KeyID`, Ed25519 signature, and proof structure. On success it stores an independently allocated copy of the public identity material, marks the initial key active, initializes rotation sequence zero, and creates membership as `PENDING`. Identity creation does not grant active membership or validator authority.

## KeyRotation and replay protection

`KeyRotation` requires the outer transaction and signed inner rotation request to match the current state's `NetworkID`. The request must identify the currently active old key and use exactly `stored_sequence + 1`. Both Phase 3 rotation proofs are verified before mutation. Success retires the old key, activates a copied new public key, advances the sequence, preserves `IdentityID`, and leaves membership unchanged.

A retired key's historical signature can remain cryptographically valid but is no longer authorized by canonical state. Replaying a prior rotation fails its sequence check; attempting a later rotation with a retired key fails the active-key check. Replay authorization therefore comes from canonical state, not from signature validity alone.

## Membership governance boundary

Direct MembershipChange remains deterministically rejected. PENDING, ACTIVE, SUSPENDED, and REVOKED remain the membership-state vocabulary; Phase 6 adds separately signed proposal, vote, finalization, and execution transactions as the only runtime authorization route. Local configuration cannot authorize a membership change.

## Deterministic resource bound

The maximum canonical transaction size is the protocol constant 65,536 bytes. Every validator measures the same canonical bytes before schema, network, type, or payload validation and returns `ERR_TRANSACTION_TOO_LARGE` when the limit is exceeded. It is not derived from RAM, operating system, environment, or per-node configuration. Broader wire-message and queue bounds belong to later networking work.

## Phase 11 registry state

StateSchemaV3 is the explicit deterministic successor to governance StateSchemaV2. It adds ID-sorted immutable Research and Resource entries with governance ProposalID and consensus execution height. Admission is dispatched only by approved `GovernanceExecute`; duplicate IDs and missing canonical Research/Resource targets fail atomically. ObjectID targets are syntax-checked without reading local object bytes. See [registries.md](registries.md).
