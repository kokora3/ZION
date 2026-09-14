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

## Phase 8 runtime, persistence, sync, and API boundaries

The versioned local API binds to loopback by default. Non-loopback binding requires explicit bearer authentication and should additionally use firewall and TLS controls. Wildcard CORS is forbidden. API responses and logs never include member, P2P, validator, or bearer-token secrets, and the three key domains remain separate.

The API accepts only bounded base64 wrappers around existing canonical signed transaction bytes. It has no administrator endpoint for membership, governance, or validator changes. Local decode, relay, or CometBFT CheckTx acceptance is not finality; only committed observation is FINALIZED.

Application snapshots store exact canonical state bytes in a separate versioned local envelope bound to NetworkID, GenesisID, accepted height, and recomputed StateHash. Input is bounded and corruption or incompatibility fails closed without an automatic destructive reset. CometBFT database files remain a distinct engine concern.

Normal-node state sync authenticates the transport peer and verifies network, GenesisID, canonical encoding, height, and StateHash before atomic persistence and in-memory promotion. It rejects rollback and equal-height conflicts and never overwrites an active validator from general P2P. This alpha mechanism is not a Byzantine-safe light client: a hash-valid snapshot plus authenticated peer does not independently prove consensus finality.

State-sync frames, relay frames, API bodies, concurrent handlers, recent transaction records, sync/relay handlers, timeouts, and shutdown are bounded. Runtime lifecycle cancellation cleans up partial startup. Operational height, sync status, API settings, transaction cache, and filesystem metadata never enter canonical StateHash.

## Phase 9 object-store and retrieval boundaries

Phase 9 reuses the frozen Phase 2 ObjectID. An ObjectID is derived from the canonical public object core and is distinct from the payload ContentHash; it is never derived from a filename, filesystem path, peer, or storage location. Stored canonical bytes are immutable. Digest-sharded paths use only a validated SHA-256 algorithm and lowercase digest, and temporary writes are bounded, owner-only where supported, flushed, and atomically renamed. Duplicate content is verified rather than overwritten. Corrupt, truncated, misplaced, malformed, non-canonical, or oversized content fails closed.

The zion-alpha-1 hard limit is 1 MiB for a complete canonical object. The local quota, object count, concurrent requests, P2P candidates, per-peer handlers, frames, deadlines, and API upload envelope are bounded. A full store rejects new content without automatic eviction and keeps existing valid objects readable. These controls reduce resource-exhaustion risk but do not establish a production capacity guarantee.

Object files, paths, quota, timestamps, cache state, peer choices, availability, and fetch failures are local operational state. They never enter canonical snapshots, membership, governance, validator authorization, transactions, `StateHash`, or `AppHash`. A corrupt store or failed fetch cannot authorize or mutate chain state.

The `/zion/object/0.1.0` protocol runs only over authenticated, compatible Phase 7 peers. PEX and remote availability are untrusted hints. Every `FOUND` response is canonically decoded, checked against declared size and ContentHash, and recomputed against the requested ObjectID before storage. Wrong-network, mismatched-ID, non-canonical, malformed, oversized, timed-out, and unsolicited responses are rejected without partial installation.

The local API accepts object bytes, never a server filesystem path. It preserves loopback defaults, bearer authentication for non-loopback binding, exact-origin CORS, bounded concurrency, and timeouts. Responses omit storage paths, credentials, private keys, and raw internal errors. The CLI validates downloaded bytes before writing and refuses accidental overwrite unless `--force` is explicit.

Content addressing proves byte integrity relative to an identifier; it does not prove authorship, endorsement, safety, availability, or permanence. The current `LOCAL` visibility label is metadata, not encryption or access control. Phase 9 provides no confidentiality, DHT, chunking, automated replication, garbage collection, or permanent-storage guarantee.

## Phase 10 signed-Board boundaries

Board events use the dedicated, NetworkID-bound `zion.board.event/v1` signing purpose. New local publication requires current ACTIVE membership and the identity's current active KeyID. A Board signature proves authorship relative to that public key; it does not grant chain, membership, governance, validator, bootstrap, or storage authority.

A retired-key or now-suspended author's historical signature may remain cryptographically valid. Remote events therefore expose signature validity, current membership, and `CURRENTLY_AUTHORIZED` versus `HISTORICAL_OR_UNCONFIRMED` independently. This current-state classification is not proof of authorization at an unrecorded historical chain height and is not perfect spam resistance or proof-of-human.

Announcement and sync origins are untrusted. Event and content objects are canonically decoded and their independent Phase 2 ObjectIDs recomputed; signatures and local chain classification are checked before an event is indexed. Announcement does not imply storage acceptance, endorsement, finality, or permanent availability. Content is fetched only after event admission and is again validated against its ObjectID and Board content schema.

Event objects, content bodies, titles, references, announce frames, sync pages, handler concurrency, fanout, queries, and search results are bounded. Board text is untrusted data. The backend neither executes HTML/script nor treats MARKDOWN as active content; any future renderer must escape or sanitize it. Malformed input must fail without panic or chain mutation.

The Board index and local hide map are rebuildable operational state. Hiding does not delete immutable objects, retract data from peers, or create global moderation authority. Public P2P content may persist on independent nodes. Phase 10 makes no confidentiality, private-group, end-to-end encryption, anonymity, guaranteed deletion, or permanent-availability claim.

Board objects, availability, index rows, search results, local hiding, announcements, and sync timing never enter canonical state. Ordinary POST/REPLY activity does not alter membership, governance, validators, transactions, `StateHash`, or `AppHash`.

## Phase 11 canonical-registry boundaries

Research and Resource admission requires the existing canonical governance process; no administrator, founder key, API, CLI, or local-config bypass exists. Entries are immutable in v0.1, duplicate admission and double execution fail deterministically, and failed execution cannot partially mutate registry or proposal state.

Apply validates only bounded canonical metadata and reference syntax/existence in canonical state. It never reads the filesystem or object store and never contacts P2P, HTTP, DNS, DOI, GitHub, or another external service. External URIs are not fetched during consensus. Missing object bytes do not invalidate an admitted ObjectID reference or change StateHash/AppHash.

ObjectID references establish content identity, not safety, endorsement, availability, or permission to execute. Registered repositories and tools are never cloned, installed, updated, or executed automatically. Governance admission is curation; it is not a malware-free, security-audited, academically peer-reviewed, or permanently available guarantee.

Canonical registry references are governance-approved chain metadata. Board Research/Resource references remain signed off-chain community statements and confer no canonical authority. Registry search indexes and `present_local` hints are replaceable local state; corrupt indexes rebuild from canonical application state without changing consensus.

## Phase 12 web-client boundaries

ZION Web is a replaceable, untrusted client of the local versioned API, not protocol authority. It does not implement or decide canonical encoding, identifiers, signatures, membership, governance, finality, Board admission, validator authorization, or P2P behavior. It requires no central database, hosted application API, analytics, advertising, session replay, or telemetry, and it never sends PeerID, IdentityID, status, or governance activity to a third party.

The browser never asks for, receives, logs, or stores member private keys or seeds. No secret may enter `NEXT_PUBLIC_*`, localStorage, or IndexedDB. Because the current runtime has no safe general member signer API, Board and governance mutations accept only already-signed canonical public payloads through existing node routes. A future node-mediated signer must remain local, enforce current authorization, and never return private keys.

The Phase 8 API remains loopback-only by default. Explicit remote mode retains bearer authentication, exact-origin CORS, firewall, and TLS responsibilities. Bearer tokens are not placed in URLs, logs, or public build variables; the official client keeps an optional token in session storage only and warns that browser-origin code can access it. Public HTTPS pages may be unable to call local HTTP because of mixed-content and private-network restrictions, so local Web plus local node is the supported alpha path.

Board text and all public metadata are untrusted. The client renders Board markup as text, enables no raw HTML, uses no untrusted `dangerouslySetInnerHTML`, and does not auto-load remote images. External registry links require explicit HTTPS navigation, expose the hostname, and use `noopener noreferrer`. Governance registration of a Resource is curation, not proof that a tool, repository, or model is safe to run; the client provides no Run or Install action.

Local hide is not deletion, client cache is not canonical, and disconnected data is labeled stale. Content may persist on independent nodes while local availability may disappear. Request timeout/cancellation, bounded pagination, restrictive response headers, and an explicit CSP reduce browser and resource risk, but are not claims of formal verification, anonymity, content safety, or permanent availability.

## Phase 13 recovery, observability, and release boundaries

Canonical state export is explicit, bounded, versioned, and secret-free. It embeds the existing canonical Snapshot bytes and binds NetworkID, GenesisID, state schema, accepted height, and recomputed StateHash. Inspection/import distrust declared metadata, reject non-canonical or corrupt bytes, and fail on wrong network/genesis, unsupported schema, oversized/truncated input, or existing destination state. Import is limited to a stopped fresh NORMAL node; it does not replace a live validator or reconcile a CometBFT database. V2-to-V3 uses the existing deterministic migration and V1 is not silently reinterpreted.

Exports exclude member, P2P, validator, and bearer secrets as well as peer cache, object availability, Board visibility, derived indexes, and Web settings. Separate key/object backups have different confidentiality and recovery requirements. A state export is not proof that off-chain objects remain available.

Structured JSON logs and Prometheus metrics are local operational output. Log level, timestamps, uptime, counters, failures, and build metadata never enter canonical state. Secrets and canonical/Board payload bodies are not logged. Metric labels are restricted to bounded network/protocol/software/sync vocabularies; identifiers, addresses, paths, content, and raw errors are forbidden labels. `/metrics` shares the API listener and its loopback default, bearer requirement for remote binding, exact CORS checks, concurrency, and timeouts.

The alpha packages are not code-signed and do not provide a secure auto-update channel. Verify SHA-256 manifests through an appropriately trusted distribution path. Dependency/license inventory and vulnerability scanning reduce known supply-chain risk but are not formal assurance or legal advice.

Final v0.1 limitations remain explicit: general state sync is not a Byzantine light client; Board/object data is public and may persist or disappear independently; registry admission is not a safety guarantee and never authorizes execution; bootstrap/PEX cannot grant authority; no central database/admin exists; key recovery is operator-managed; and `zion-alpha-1` may reset with an explicit migration boundary.
# Distribution and container boundary

D1 container images contain no member, P2P, API bearer, or validator private secrets. P2P identity and the Compose API token are created at runtime in persistent storage; validator genesis/key/state and node key are provisioned externally. `/var/lib/zion` remains outside the replaceable container, while configuration is mounted read-only at `/etc/zion/zion.yaml`.

Compose publishes the API/metrics and Web ports only on host loopback by default. A bootstrap profile publishes general P2P UDP only; a validator additionally publishes CometBFT P2P TCP and keeps RPC disabled. Remote API exposure still requires bearer authentication, exact CORS, firewalling, and TLS. The public Web bundle receives only `NEXT_PUBLIC_ZION_API_URL`, never a token or private key.

The node runtime uses non-root UID/GID 10001, a read-only root filesystem, bounded tmpfs, no added Linux capabilities, and `no-new-privileges`; it is never privileged and uses neither host PID nor host networking. Docker is packaging and process isolation, not a protocol trust boundary. A profile cannot grant membership, bootstrap truth, governance rights, or validator authority. Named volumes are role-specific, and operators must never mount one mutable directory into concurrent nodes.
