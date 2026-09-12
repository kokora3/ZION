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

Membership authority cannot come from local configuration. Direct MembershipChange remains disabled; Phase 6 permits only a canonically approved governance proposal to execute a membership transition. Private keys and Ed25519 seeds never enter canonical State or Snapshot; only public protocol material is stored.

The canonical transaction limit is the fixed protocol value 65,536 bytes and is enforced identically by every validator before other transaction validation. Consensus resource limits must be deterministic and bounded rather than derived from RAM, the operating system, environment variables, or unique local configuration. These controls and tests reduce risk but are not a claim of formal verification.

## Phase 5 consensus boundaries

ZION uses CometBFT rather than inventing a BFT algorithm. The engine orders transactions, runs voting rounds, and establishes finality; it does not define ZION application authorization. Every finalized transaction is decoded at the canonical ZION wire boundary and revalidated in block order through the Phase 4 state machine. Check/mempool acceptance is non-mutating and is never final authorization.

Validator consensus keys are a separate cryptographic identity domain from member identity keys and future general-network PeerIDs. Consensus private keys never enter canonical ZION state, protocol fixtures, or source control. Deterministic keys in tests are marked **TEST ONLY — PUBLIC FIXTURE — NOT SECRET — NEVER USE IN PRODUCTION**.

Malformed, oversized, wrong-network, unsupported, or non-canonical consensus-input transactions fail safely and cannot partially mutate state. Proposal processing evaluates state-dependent transactions in their supplied order. Finalized invalid transactions have deterministic results and leave state unchanged. CometBFT `AppHash` is exactly the raw SHA-256 digest carried by the Phase 4 `StateHash`; validators must agree on both.

The alpha validator set is permissioned at genesis, contains four distinct equal-power consensus keys, and requires the engine's greater-than-two-thirds voting threshold. Loss of quorum stops liveness instead of permitting unsafe finalization. Runtime changes require an executed canonical governance proposal and are emitted through ABCI; direct transactions and local configuration cannot authorize them. CometBFT databases and wire messages are engine implementation details, not ZION canonical state or compatibility formats. ZION does not claim to have formally verified CometBFT.

## Phase 6 governance boundaries

There is no governance superuser or local-admin bypass after deterministic genesis bootstrap. A governance vote is not a CometBFT consensus vote: governance gives one vote to each eligible ACTIVE IdentityID, independent of validator power, tokens, stake, reputation, identity age, or machine resources. PENDING, SUSPENDED, and REVOKED identities cannot vote.

Proposal, vote, finalization, and execution signatures use separate network-bound domains. Votes are keyed by IdentityID, so rotating a member signing key cannot create a second vote. A proposal snapshots its sorted ACTIVE electorate when opened and still requires current ACTIVE authorization at vote time. Integer strict-two-thirds arithmetic avoids floating-point nondeterminism; ABSTAIN counts for the minimum three participants but is excluded from YES+NO.

Execution revalidates current canonical state. Stale membership or validator-set preconditions fail atomically, and an approved proposal cannot execute twice. Validator changes require canonical approval, keep operator IdentityID separate from the consensus key, retain equal power 1, and cannot reduce the validator set below three.

CheckTx never mutates proposal, vote, membership, or validator state; finalized ordered execution performs authorization again. Chain height, not wall clock, determines voting and finalization boundaries. Local time, filesystem state, APIs, randomness, and scheduling are not governance inputs.

Phase 6 bounds canonical transactions at the existing 65,536-byte protocol limit, proposal payloads at 4,096 canonical bytes, and electorates/votes at 1,024 per proposal. Finalized proposal history remains canonical and is not pruned in this phase. These controls are not a claim of formal verification.

## Phase 7 general-P2P boundaries

The libp2p PeerID is cryptographic transport identity; DNS names, IP addresses, ports, and multiaddrs are locations. A dial address includes the expected PeerID, and the ZION hello's reported PeerID must equal the remote identity authenticated by libp2p. Member, CometBFT validator, and P2P private keys are separate. P2P keys never enter canonical state, and changing them cannot alter IdentityID, membership, governance authority, validator identity, `StateHash`, or `AppHash`.

Bootstrap peers and DNS are replaceable discovery infrastructure, not trust or protocol authority. PEX records are untrusted candidate contact information. Every candidate is independently transport-authenticated and must pass the ZION network, existing genesis fingerprint, typed version, role, address, and canonical-CBOR hello checks. A claimed `VALIDATOR` role is only a capability hint and cannot change the canonical CometBFT validator set; a claimed `BOOTSTRAP` role grants no privilege.

Peer-cache JSON is bounded local operational state. Its timestamps and dial failures never enter transactions or canonical state. A corrupt, oversized, or compromised cache cannot authorize chain changes and fails safely to an empty cache. Private keys, member secrets, validator secrets, credentials, and filesystem metadata are never advertised by hello or PEX.

ZION-owned hello and PEX frames have fixed size, count, nesting, role, PeerID, and multiaddr bounds. Non-canonical or malformed input is rejected without promotion to the usable peer set. Connected peers, concurrent dials, cache records, addresses, retries, and PEX output are bounded; contexts, deadlines, and capped backoff prevent immediate unbounded redial loops. These measures reduce denial-of-service risk but do not provide Sybil resistance.

An outbound-only NORMAL node can participate without public inbound reachability. Phase 7 deliberately does not provide AutoNAT, hole punching, DCUtR, UPnP, NAT-PMP, relay, TURN, or DHT behavior and makes no universal NAT-connectivity or anonymity claim.
