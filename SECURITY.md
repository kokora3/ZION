# Security Policy

ZION alpha software is experimental. Do not assume production security or use it for security-critical assets or data.

Please do not publicly disclose suspected security vulnerabilities before coordination. Prefer a GitHub private security advisory for this repository when available. No security-reporting email address is currently published.

Changes involving cryptography, consensus, canonical encoding, identity, or parsers require additional review.

## Phase 3 identity boundaries

Private keys never appear in public protocol objects, logs, or public state exports. Deterministic fixture keys are **TEST ONLY — PUBLIC FIXTURE — NOT SECRET — NEVER USE IN PRODUCTION**.

A valid signature is not current authorization. Historical signatures from retired keys may remain cryptographically valid, while Phase 4 canonical chain state will decide which keys are currently authorized. Normal rotation requires old-key authorization and new-key proof of possession; its signed sequence alone does not reject a valid historical replay.

Valid identity does not imply ACTIVE membership, and ACTIVE membership does not imply validator authority. A member IdentityID is not a P2P Peer ID. Unknown algorithms fail closed; malformed keys, IDs, signatures, and encoded structures must fail safely without panic. This is not a claim of formal verification.

## Phase 4 deterministic-state boundaries

Consensus state must be deterministic. The chain transition receives only the previous canonical state and transaction; local wall clock values, random values, external network or API state, and filesystem ordering are not transition inputs. Go map iteration order, pointer identity, allocation layout, and slice capacity cannot affect canonical snapshot bytes or `StateHash`.

Transactions are applied copy-on-write. A failed transaction must not partially mutate identity records, active keys, rotation sequence, or membership, and its receipt reports the unchanged pre-transaction `StateHash`. Receipts exclude local time, randomness, memory addresses, stack traces, goroutine identifiers, and other process-local data.

Cryptographically valid does not mean currently authorized. Canonical state enforces the next rotation sequence and the currently active old key, so replayed rotations and later authorization attempts by retired keys fail even when their historical signatures remain valid.

Membership authority cannot come from local configuration. `MembershipChange` remains disabled until protocol governance authorization is specified. Private keys and Ed25519 seeds never enter canonical `State` or `Snapshot`; only public protocol material is stored.

The canonical transaction limit is the fixed protocol value 65,536 bytes and is enforced identically by every validator before other transaction validation. Consensus resource limits must be deterministic and bounded rather than derived from RAM, the operating system, environment variables, or unique local configuration. These controls and tests reduce risk but are not a claim of formal verification.

## Phase 5 consensus boundaries

ZION uses CometBFT rather than inventing a BFT algorithm. The engine orders transactions, runs voting rounds, and establishes finality; it does not define ZION application authorization. Every finalized transaction is decoded at the canonical ZION wire boundary and revalidated in block order through the Phase 4 state machine. Check/mempool acceptance is non-mutating and is never final authorization.

Validator consensus keys are a separate cryptographic identity domain from member identity keys and future general-network PeerIDs. Consensus private keys never enter canonical ZION state, protocol fixtures, or source control. Deterministic keys in tests are marked **TEST ONLY — PUBLIC FIXTURE — NOT SECRET — NEVER USE IN PRODUCTION**.

Malformed, oversized, wrong-network, unsupported, or non-canonical consensus-input transactions fail safely and cannot partially mutate state. Proposal processing evaluates state-dependent transactions in their supplied order. Finalized invalid transactions have deterministic results and leave state unchanged. CometBFT `AppHash` is exactly the raw SHA-256 digest carried by the Phase 4 `StateHash`; validators must agree on both.

The alpha validator set is permissioned at genesis, contains four distinct equal-power consensus keys, and requires the engine's greater-than-two-thirds voting threshold. Loss of quorum stops liveness instead of permitting unsafe finalization. Runtime validator-set changes remain disabled until governance authorization exists. CometBFT databases and wire messages are engine implementation details, not ZION canonical state or compatibility formats. ZION does not claim to have formally verified CometBFT.
