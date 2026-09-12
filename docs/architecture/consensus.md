# BFT Consensus Integration

## Role and boundary

ZION v0.1 uses CometBFT v1.0.1, licensed Apache-2.0, as its reference consensus engine. A mature engine supplies validator transport, proposer selection, voting rounds, ordering, and deterministic BFT finality. ZION does not implement its own prevote, precommit, locking, timeout, evidence, or fork-choice rules.

This is not Cosmos SDK. The repository imports CometBFT directly and does not use Cosmos accounts, modules, staking, coins, governance, IBC, or application state. ZION continues to own canonical CBOR transactions, `TxID`, identities, membership, authorization, the native state machine, snapshots, and `StateHash`.

```text
CometBFT ordered transaction bytes
                |
                v
       ZION consensus adapter
                |
                v
        Phase 4 chain.Apply
                |
                v
       Canonical ZION State
                |
                v
   StateHash digest == AppHash
```

CometBFT never mutates identity or membership records directly. All successful canonical transitions pass through `internal/chain`.

## Identity domains

Three identity domains remain distinct:

1. A ZION member `IdentityID` identifies authorship, membership, and future application governance.
2. A CometBFT validator consensus key/address authenticates BFT votes.
3. A future ZION/libp2p `PeerID` will identify general content-network peers in Phase 7.

A future mapping may associate a validator with a member, but their keys and meanings remain separate. Validator private keys are local engine secrets and never enter canonical ZION state. Test keys are public fixtures and cannot be used in production.

## Permissioned alpha genesis

The local alpha model has exactly four validators with distinct Ed25519 consensus keys and equal voting power of one. Validator definitions are sorted by public key before their set hash is derived, so configuration insertion order is irrelevant. Duplicate keys, duplicate names, invalid key lengths, unequal power, and counts other than four are rejected.

The application genesis document binds the `NetworkID`, empty Phase 4 `StateHash`, and validator-set hash. Its digest contributes to the CometBFT chain ID. The adapter independently checks chain ID, application genesis fields, and validator updates during `InitChain`. A wrong-network or wrong-genesis node therefore cannot silently participate in the alpha validator network.

CometBFT's greater-than-two-thirds rule means three of four equal-power validators can finalize and two cannot. One offline validator preserves liveness. Losing a second stops new finality without changing committed application state. Restoring a third validator resumes finality using isolated engine storage and the retained application state.

## Transaction wire boundary

`DecodeTransaction` enforces the frozen 65,536-byte raw/canonical transaction limit before decoding. It uses the strict Phase 2 CBOR decoder, rejects unsupported schema, wrong network, unknown or reserved transaction type, and mismatched payload shape, then canonical re-encodes the transaction and requires byte-for-byte equality. Malformed input never reaches application semantics and cannot panic the adapter. The ZION-owned raw decoder has dedicated fuzz coverage; CometBFT wire messages are not ZION fixtures.

`CheckTx` performs the same decode and evaluates a transaction against the current committed state without publishing the resulting state. Acceptance only means the transaction was valid at that moment. It can become invalid before finalization.

`PrepareProposal` preserves mempool order while deterministically filtering transactions that fail sequential evaluation or exceed the proposal byte allowance. `ProcessProposal` replays the proposed list in order on a temporary state and rejects the complete proposal if any transaction is invalid. This protects against Byzantine proposer input.

`FinalizeBlock` performs a final independent decode and sequential evaluation through `chain.Apply`. A malformed or invalid transaction receives a stable ABCI code/data result and leaves the working state unchanged. Successful results advance the working state. `Commit` publishes only the fully finalized working state. Parser error strings, time, randomness, addresses, and process-local details are excluded from consensus-visible results.

## AppHash, height, and persistence

The CometBFT `AppHash` byte sequence is exactly `StateHash.HashDigest.Digest`: the 32 raw SHA-256 bytes of the canonical Phase 4 snapshot. It is not a second application hash. Four independent applications given the same ordered transactions must produce identical results, snapshot bytes, `StateHash`, and `AppHash`.

Consensus height is adapter metadata outside canonical ZION state. It is not a rotation sequence, schema version, or protocol version and does not alter Phase 4 bytes. Phase 5 keeps application state in memory. Integration tests use temporary CometBFT directories to exercise restart/recovery; the engine database is not a ZION persistence or canonical-state format.

The initial validator set comes from bound genesis configuration. Phase 6 can later emit runtime ADD/REMOVE updates only after canonical governance approval; CometBFT v1.0.1 makes an update returned at height H effective at H+2. General bootstrap, discovery, DHT, NAT traversal, relaying, and normal-node P2P remain outside this layer; CometBFT validator transport is not the future ZION content network.
