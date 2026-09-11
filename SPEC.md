# ZION Protocol v0.1 — Minimum Decentralized Product
## Network: `zion-alpha-1`

**Status:** Draft / Implementation Baseline  
**Protocol version:** `0.1`  
**Release goal:** Minimum Decentralized Product  
**Network class:** Alpha network — reset/migration is explicitly allowed  
**Primary implementation:** Go Native ZION Node + replaceable clients  
**Official interface:** ZION Web / Next.js  
**Economic model:** No coin/token; ZION Points are deferred beyond v0.1  

---

# 0. Normative language

The words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are normative.

- **MUST / MUST NOT**: required for compatibility or security.
- **SHOULD / SHOULD NOT**: strongly recommended; exceptions require justification.
- **MAY**: optional implementation choice.

---

# 1. Purpose

`zion-alpha-1` is the first usable product slice of ZION.

The goal of v0.1 is not to implement the full long-term ZION vision.

The goal is to prove that several independent ZION nodes can:

1. form a network without a mandatory central application database;
2. maintain shared trusted chain state;
3. identify and authenticate members cryptographically;
4. exchange signed community content;
5. curate AI-security research and resources;
6. approve or reject canonical entries through simple governance;
7. expose the shared network through a usable web interface.

The core product question is:

> **Can several independent ZION nodes form one network, maintain shared trusted state, and let humans discuss and curate AI-security knowledge without a central application database?**

If the answer is yes, ZION v0.1 succeeds.

The core architecture rule remains:

> **The chain stores trust, compact canonical state, registries, governance decisions, and references. Content payloads live outside the chain.**

---

# 2. Product scope

## 2.1 In scope for v0.1

```text
✓ Go Native ZION Node
✓ Replaceable Next.js frontend
✓ Basic zionctl command-line client
✓ Permissioned alpha membership
✓ Cryptographic identity
✓ Key rotation
✓ Minimal deterministic native chain state machine
✓ Known BFT design / deterministic finality
✓ Permissioned validator set
✓ Simple governance
✓ P2P peer discovery
✓ Bootstrap peers
✓ Direct peer connections
✓ Normal-peer outbound participation from behind NAT
✓ Peer cache + configured/DNS bootstrap seeds + manual peer override
✓ Explicit v0.1 node roles: NORMAL / VALIDATOR / BOOTSTRAP
✓ Versioned P2P protocol
✓ Signed Board posts and replies
✓ Research metadata registry
✓ Resource metadata registry
✓ Simple content-addressed small-object storage
✓ Basic object references
✓ Local derived index/search
✓ Resource governor / hard safety limits
✓ Local versioned Node API
✓ State export/import
✓ Basic metrics/observability
✓ Multi-node acceptance testing
```

## 2.2 Explicitly deferred beyond v0.1

```text
✗ ZION Points (ZP)
✗ Proposal deposits
✗ Slashing
✗ Reputation-weighted voting
✗ Automatic reward policy

✗ NAT hole punching
✗ AutoNAT / DCUtR
✗ Community relay network
✗ Advanced relay selection

✗ Content-defined chunking
✗ Large-file distributed chunk storage
✗ Advanced deduplication
✗ Replication targets
✗ Pin/unpin policy
✗ Automatic garbage collection

✗ Git-like repository snapshots
✗ Automatic upstream watcher
✗ Repository ingestion sandbox
✗ Distributed CDN
✗ Package installation

✗ Canonical knowledge graph subsystem
✗ Extensible relation namespaces
✗ Graph governance

✗ Advanced moderation lifecycle
✗ Advanced anti-spam reputation model

✗ Guardian key recovery
✗ Advanced identity recovery

✗ ZION Desktop / Tauri

✗ Advanced private groups / E2EE
✗ Proof-of-human
✗ Hardware attestation / TPM / Secure Enclave
✗ Coin/token economics
✗ Open proof-of-stake validators
✗ Novel consensus
✗ Novel cryptography
```

---

# 3. Alpha status

The network identifier MUST be:

```text
zion-alpha-1
```

`zion-alpha-1` MUST NOT be marketed as mainnet.

Users MUST be informed that:

- chain history may be reset;
- validator membership may change;
- a new genesis may be created;
- canonical state may migrate to a later alpha;
- protocol rules may change;
- no economic asset is issued in v0.1.

Before external alpha users are invited, the node MUST support state export/import or equivalent migration.

---

# 4. Constitutional invariants

The following rules define the minimum architecture of ZION v0.1:

```text
1. No mandatory central application server.
2. No mandatory central application database.
3. No single protocol superuser.
4. Official clients are replaceable.
5. The chain stores compact trusted state, not bulk content.
6. Protocol objects are integrity-verifiable.
7. Public distributed content may not be globally erasable after distribution.
8. Resource use must be bounded before data is accepted.
9. Protocol formats must be explicitly versioned.
10. Identity keys must support rotation/revocation.
11. Consensus state must be deterministic.
12. Alpha state must be exportable and migratable.
```

A `CONSTITUTION.md` SHOULD contain these principles.

---

# 5. High-level architecture

```text
                           ZION v0.1

                     ┌──────────────────┐
                     │    ZION Chain    │
                     │                  │
                     │ Identity         │
                     │ Membership       │
                     │ Governance       │
                     │ Research Registry│
                     │ Resource Registry│
                     │ Object References│
                     │ Checkpoints      │
                     └────────┬─────────┘
                              │
            ┌─────────────────┼─────────────────┐
            │                 │                 │
            ▼                 ▼                 ▼
        COMMUNITY          RESEARCH          RESOURCES
        Board              Papers            Repositories
        Replies            Metadata          Tools
        References         Sources           Datasets
            │                 │                 │
            └─────────────────┼─────────────────┘
                              ▼
                  Content-Addressed Objects
                     small payloads only
                              │
                              ▼
                         P2P Network
                   Discovery / QUIC / Bootstrap
                              │
                    ┌─────────┼─────────┐
                    ▼         ▼         ▼
                  Node A    Node B    Node C
                              │
                              ▼
                        Local Indexer
                              │
                              ▼
                         ZION Node API
                              │
                     ┌────────┴────────┐
                     ▼                 ▼
                  ZION Web          zionctl
```

---

# 6. ZION Node

The **ZION Node** is the protocol application.

The v0.1 reference implementation SHOULD be written in Go.

The Go implementation is the reference implementation, but protocol compatibility MUST be defined by this specification rather than by Go-specific behavior.

Future implementations in another language MUST be able to reproduce the same canonical protocol objects and deterministic chain state.

## 6.1 Responsibilities

A v0.1 node is responsible for:

- local identity and key management;
- membership state;
- chain synchronization or validation;
- validator participation when authorized;
- P2P peer discovery;
- signed event validation;
- small content-addressed object storage;
- Board synchronization;
- Research Registry synchronization;
- Resource Registry synchronization;
- governance voting;
- local derived indexing;
- resource limits;
- local API exposure.

## 6.2 Official programs

```text
zion-node    Go daemon / core node process
zionctl      Go command-line control client
ZION Web     Replaceable Next.js frontend
```

ZION Web and `zionctl` MUST NOT implement independent chain or P2P stacks.

They communicate with the same local `zion-node`.

## 6.3 v0.1 node roles

v0.1 implements three explicit operational roles:

```text
NORMAL
VALIDATOR
BOOTSTRAP
```

Roles are capabilities, not mutually exclusive node types.

A single node MAY operate as:

```text
NORMAL + VALIDATOR + BOOTSTRAP
```

### NORMAL

A normal node:

- synchronizes chain state;
- participates in the P2P network;
- exchanges Board and registry content;
- maintains local derived indexes;
- uses the local API and official clients.

A NORMAL node does not require inbound Internet reachability in v0.1.

### VALIDATOR

A validator is a NORMAL node that is additionally authorized by genesis/chain state to participate in consensus.

A local configuration toggle MUST NOT grant validator authority by itself.

### BOOTSTRAP

A bootstrap node is a reachable peer used to help a new node enter the network and discover additional peers.

A bootstrap node:

- is not an application source of truth;
- MUST NOT be required in every data path;
- MUST be replaceable;
- SHOULD be publicly reachable.

### Reserved future roles

The following role names are reserved for later versions and are not implemented as full v0.1 subsystems:

```text
STORAGE
RELAY
```

`STORAGE` is reserved for future large-object replication/preservation.

`RELAY` is reserved for future NAT/CGNAT relay connectivity.

---

# 7. Node configuration

The reference implementation SHOULD use a human-readable configuration file such as:

```text
zion.yaml
```

or:

```text
zion.toml
```

The exact format is an implementation choice, but configuration MUST be validated before use.

Minimum v0.1 controls SHOULD include:

```text
network_id
data_directory
listen_addresses
bootstrap_peers
max_peers
max_storage_gb
reserve_free_gb
download_limit_mbps
upload_limit_mbps
api_bind_address
validator_enabled
```

A normal user MUST NOT become a validator merely by toggling a local configuration value.

Validator authorization is governed by chain/genesis state.

The local API MUST bind to:

```text
127.0.0.1
```

by default.

## 7.1 Resource-efficiency targets

The reference implementation SHOULD remain practical on inexpensive commodity hardware.

The following are **engineering targets**, not consensus rules or hard protocol guarantees:

```text
Idle CPU:       low single-digit percentage where practical
Idle RAM:       target < 512 MiB
Normal node:    target < 1 GiB RAM under ordinary alpha load
Validator:      target < 1 GiB RAM under ordinary alpha load
Disk usage:     bounded by configured limits
Queues/caches:  bounded
Peer count:     configurable
```

These targets MAY be revised after measurement and benchmarking without changing protocol compatibility.

If a lightly loaded alpha node exceeds these targets, the implementation SHOULD be profiled and optimized before simply requiring larger hardware.

Resource efficiency is also a security property: hostile peers MUST NOT be able to force unbounded CPU, memory, disk, connection, or queue growth.

---

# 8. Identity

## 8.1 Identity record

Each member has a stable cryptographic identity.

Minimum fields:

```text
identity_id
active_signing_keys[]
key_algorithm
created_at
revocation_state
key_rotation_state
```

The schema MUST NOT permanently assume one signing algorithm.

## 8.2 Crypto agility

Cryptographic data MUST be self-describing.

Store:

```text
algorithm_id
public_key
signature
```

Hashes SHOULD also identify the hash algorithm.

## 8.3 Key rotation

An identity MUST be able to rotate an active signing key.

Normal rotation SHOULD require authorization by a currently valid key.

Advanced guardian/governance recovery is deferred.

## 8.4 Membership

Cryptographic identity does not equal proof-of-human.

For v0.1:

- anyone MAY run a non-governance node;
- governance membership is permissioned;
- validator membership is permissioned.

Minimum states:

```text
PENDING
ACTIVE
SUSPENDED
REVOKED
```

---

# 9. ZION Chain

## 9.1 Purpose

The chain is the compact trusted state layer.

The chain SHOULD store:

- identity state;
- membership state;
- governance proposals;
- governance votes;
- accepted Research Registry entries;
- accepted Resource Registry entries;
- accepted object references;
- protocol policy/version information;
- validator set changes;
- checkpoints.

The chain MUST NOT store:

- full PDFs;
- Git repositories;
- large datasets;
- large binaries;
- full large attachments.

## 9.2 Native chain

ZION v0.1 uses a native chain/state machine.

This means ZION owns:

```text
transaction model
state transition rules
validator authorization rules
chain data model
governance model
registry model
node implementation
```

However, ZION v0.1 MUST NOT invent a novel consensus algorithm.

## 9.3 Consensus

Use a known BFT family/design with deterministic finality.

Recommended alpha properties:

```text
Initial validators: 4
Validator set: permissioned
Finality: deterministic
Quorum: >2/3
PoS: none
Coin-based voting: none
```

The exact BFT implementation MAY change before genesis freeze.

ZION MUST NOT claim an unreviewed custom BFT implementation is secure.

## 9.4 Minimum transaction families

```text
IdentityCreate
MembershipChange
KeyRotation

GovernanceProposal
GovernanceVote

ResearchRegister
ResourceRegister

BoardReference

ValidatorSetChange
ProtocolUpgrade
Checkpoint
```

Every chain transaction MUST be signed and deterministically validated.

---

# 10. Deterministic state machine

Given the same:

```text
previous_state
+
transaction
```

all honest validators MUST produce the same result.

Consensus state MUST NOT depend on:

- local filesystem ordering;
- local random APIs;
- external HTTP requests;
- mutable GitHub/API responses;
- process-specific map iteration order;
- local wall clock except where deterministic timestamp rules explicitly permit it.

State transition tests MUST include deterministic replay.

---

# 11. Canonical encoding

Canonical encoding is a hard protocol boundary.

Protocol objects MUST have deterministic canonical bytes.

Recommended approach:

```text
Canonical CBOR for protocol objects
JSON for client/API representation
```

Canonical rules MUST define:

- field ordering;
- integer encoding;
- byte encoding;
- Unicode normalization;
- timestamp representation;
- map ordering;
- optional/absent fields;
- schema version.

Two compatible nodes encoding the same logical protocol object MUST produce identical canonical bytes.

Go map iteration order MUST NEVER be allowed to influence canonical bytes.

---

# 12. Object identity

Database auto-increment IDs MUST NOT be protocol identities.

Objects SHOULD use integrity-verifiable identifiers.

Conceptual form:

```text
zion:obj:<algorithm>:<digest>
```

Hash algorithm identifiers MUST be explicit.

The exact human-readable encoding MUST be frozen before public alpha genesis.

---

# 13. Minimal content object

v0.1 uses a simplified content envelope.

Minimum fields:

```text
object_id
object_type
schema_version
creator_identity
created_at
content_hash
hash_algorithm
size_bytes
visibility
metadata
signature_algorithm
signature
```

Optional fields MAY include:

```text
mime_type
source_urls
logical_object_id
parent_object_id
references[]
```

Visibility MUST distinguish at least:

```text
PUBLIC
LOCAL
```

---

# 14. Small-object storage

v0.1 intentionally avoids building the full distributed storage subsystem.

The node MAY store small content-addressed payloads locally and fetch them from peers.

Suitable v0.1 content includes:

```text
Board posts
Replies
Research metadata
Resource metadata
Small text attachments
Small JSON/CBOR metadata objects
```

Large payload preservation is deferred.

The implementation MUST define:

```text
maximum object size
maximum metadata size
maximum local storage
minimum free-disk reserve
```

A remote peer MUST NOT be able to force unlimited disk growth.

Large-object chunking, replication policies, pinning, and GC are deferred.

---

# 15. P2P network

## 15.1 Discovery

ZION MUST NOT require a central authoritative routing table.

v0.1 SHOULD support:

- QUIC or equivalent modern transport;
- peer identity;
- persisted peer caching;
- configured bootstrap peers;
- DNS-based bootstrap seeds where practical;
- manually supplied peer addresses;
- version negotiation;
- bounded connection queues;
- retry/backoff.

A Kademlia-style DHT MAY be used.

A node SHOULD use the following startup discovery order:

```text
1. persisted peer cache
2. configured or DNS bootstrap seeds
3. static fallback bootstrap addresses
4. manually supplied peer address
```

If a previously known peer is reachable, a node MAY join through that peer without contacting an official bootstrap service.

Bootstrap addresses are discovery configuration and MUST NOT become immutable consensus state.

## 15.2 Bootstrap peers

Default bootstrap peers MAY ship with the official node or be resolved through a stable DNS seed name.

Bootstrap peers:

- help new nodes find the network;
- are not the application source of truth;
- MUST be replaceable;
- MUST NOT remain mandatory in every data path;
- SHOULD advertise a cryptographically verifiable peer identity.

Recommended alpha:

```text
Bootstrap peers: 2–4
Independent operators: at least 2 where possible
```

A bootstrap endpoint SHOULD be representable using a stable hostname plus transport and peer identity, for example conceptually:

```text
/dns4/bootstrap-zion.example/udp/42000/quic-v1/p2p/<peer-id>
```

The hostname tells a node where to connect; the peer identity tells it which cryptographic peer it expects.

A DNS/IP address alone MUST NOT be treated as sufficient peer identity.

Changing bootstrap infrastructure SHOULD NOT require a chain reset or genesis change.

## 15.3 NAT limitations and outbound participation in v0.1

v0.1 does not promise universal inbound peer connectivity behind arbitrary NAT/CGNAT.

Hole punching, AutoNAT, DCUtR, and relay infrastructure are deferred.

However, a normal v0.1 node behind ordinary NAT MUST be able to participate in the network by initiating outbound connections to reachable peers.

Inbound reachability is NOT required for normal v0.1 participation.

For alpha:

- public validators/bootstrap nodes SHOULD be directly reachable;
- normal nodes MAY remain behind NAT and connect outward;
- normal nodes SHOULD be able to sync chain state, exchange Board content, use registries, and submit governance/chain actions through established outbound peer connections;
- test environments MAY use VPS/public nodes;
- CGNAT users are not guaranteed to accept unsolicited inbound peer connections until relay/hole-punching support is implemented.

This limitation MUST be documented clearly.

---

# 16. Protocol negotiation

Every peer handshake MUST advertise:

```text
network_id
protocol_version
supported_object_versions
supported_features
transport_capabilities
```

Incompatible nodes MUST fail safely.

Protocol identifiers SHOULD be explicitly versioned, for example:

```text
/zion/alpha/1/...
```

---

# 17. Community Board

The Board is the first visible user-facing decentralized feature.

## 17.1 Publishing

An ACTIVE member MAY publish a signed Board post.

Minimum fields:

```text
post_id
author
created_at
content_object
references[]
signature
```

Replies use the same signed-event model.

Board payloads are exchanged through P2P content/event synchronization.

They are not stored in full on chain.

## 17.2 Authorship

The author signature MUST be verifiable.

Other nodes MUST NOT rewrite signed authorship.

## 17.3 Basic moderation

v0.1 MAY support simple local/client moderation such as:

```text
hide locally
mark spam locally
block local display
```

A simple governance-based membership suspension MAY exist.

Advanced distributed moderation lifecycle is deferred.

---

# 18. Research Registry

The v0.1 Research Registry stores metadata and references, not a full distributed paper archive.

Minimum fields SHOULD include:

```text
title
authors
DOI / arXiv / canonical source
abstract or summary metadata
topic/category
source URL(s)
license_status
proposer
signature
```

License states SHOULD include:

```text
REDISTRIBUTABLE
REFERENCE_ONLY
UNKNOWN
```

In v0.1, storing metadata/reference is sufficient.

Automatic PDF mirroring is not required.

## 18.1 Admission

Suggested workflow:

```text
Proposal
  ↓
Basic validation
  ↓
Duplicate check
  ↓
Governance vote
  ↓
Accepted / Rejected
```

Accepted entries become canonical Research Registry state.

---

# 19. Resource Registry

The v0.1 Resource Registry stores metadata for repositories, tools, datasets, and artifacts.

Minimum fields SHOULD include:

```text
resource_type
name
description
upstream_url
upstream_version_or_commit
license
source_metadata
proposer
signature
```

Possible types:

```text
Repository
Tool
Dataset
ArtifactReference
```

v0.1 does not archive full repositories automatically.

It does not run repository code.

It does not implement a package manager.

## 19.1 Resource status

v0.1 SHOULD distinguish at least:

```text
DISCOVERED
CURATED
```

`ARCHIVED`, `VERIFIED`, and `RECOMMENDED` MAY be added later when preservation and supply-chain verification are implemented.

---

# 20. Simple references

v0.1 does not implement the full Knowledge / Resource Graph.

Objects MAY contain:

```text
references[]
```

Example:

```text
BoardPost → Paper
BoardPost → Repository
Paper → Dataset
Resource → Paper
```

The local index MAY derive backlinks.

Canonical graph edges, relation namespaces, and graph governance are deferred.

---

# 21. Governance

v0.1 governance MUST remain intentionally simple.

Every ACTIVE governance member has:

```text
1 vote
```

No ZP-based weighting exists in v0.1.

A proposal is approved using:

```text
minimum unique eligible voters
+
approval threshold
```

Recommended alpha default:

```text
Minimum eligible voters: 3
Approval threshold: >2/3
```

These values are policy defaults and MAY change before genesis freeze.

Governance SHOULD cover:

- membership admission;
- membership suspension/revocation;
- Research Registry admission;
- Resource Registry admission;
- validator-set changes;
- protocol upgrades;
- migration decisions.

Voting itself does not create rewards.

---

# 22. Resource Governor

Resource exhaustion is a first-class threat even in the minimum product.

The node MUST enforce bounded resources.

At minimum:

```text
maximum peer count
maximum concurrent streams
maximum pending requests
maximum message size
maximum object size
maximum metadata size
bounded in-memory queues
per-peer rate limits
request timeouts
connection backoff
maximum local storage
free-disk reserve
```

When limits are exceeded:

```text
reject / drop / defer
```

is preferred over unbounded memory or disk growth.

The implementation MUST NOT rely on AI/human classification as the security boundary.

---

# 23. Local index and search

Each node SHOULD maintain a local derived index for:

- Board text/search;
- Research metadata search;
- Resource metadata search;
- references/backlinks;
- local moderation state.

The index is not consensus truth.

It MUST be rebuildable from:

```text
canonical chain state
+
available signed P2P objects
```

Deleting the local index MUST NOT destroy canonical network state.

---

# 24. ZION Node API

The node MUST expose a stable versioned local API.

Suggested namespace:

```text
/api/v1/...
```

Minimum groups:

```text
/status
/identity
/membership
/board
/research
/resources
/governance
/objects
/peers
/search
```

The API MUST distinguish:

- canonical chain state;
- local derived/index state;
- unconfirmed P2P content.

Default bind:

```text
127.0.0.1
```

Remote administration is not required in v0.1.

---

# 25. ZION Web

ZION Web is the primary visible product interface for v0.1.

It is a Next.js client of the local ZION Node API.

It MUST NOT:

- contain authoritative protocol state;
- require a central production database;
- become mandatory for protocol operation.

## 25.1 Minimum pages

```text
Home / Network Overview
Board
Research
Resources
Governance
Network / Peers
Identity
```

## 25.2 Minimum overview

The UI SHOULD expose:

```text
Network ID
Node status
Chain height
Peer count
Identity status
Validator/non-validator status
Recent Board activity
Recent Research entries
Recent Resource entries
Open governance proposals
```

The design MAY remain minimal.

The priority is a usable product slice, not final visual polish.

---

# 26. zionctl

v0.1 SHOULD include a small CLI.

Minimum commands:

```text
zionctl status
zionctl peers
zionctl identity show
zionctl governance list
zionctl research list
zionctl resources list
```

Optional commands:

```text
zionctl start
zionctl stop
zionctl config show
```

The CLI MUST work without ZION Web.

---

# 27. State export/import

Before inviting external alpha users, the node MUST support canonical state export/import.

Export SHOULD include:

```text
identities
membership
validator set
governance decisions
Research Registry
Resource Registry
protocol version
checkpoints / required canonical state
```

Private keys MUST NOT be included in public state export.

A migration path to a later alpha MUST remain possible.

---

# 28. Observability

The reference node SHOULD expose local metrics for:

```text
peer count
chain height
validator status
bandwidth
invalid signatures
invalid objects
rate-limit drops
queue saturation
API health
storage used
```

Observability MUST NOT require sending private telemetry to a central ZION service.

---

# 29. Recommended repository layout

```text
zion/
├── go.mod
├── go.sum
├── README.md
├── SPEC.md
├── CONSTITUTION.md
├── SECURITY.md
├── CONTRIBUTING.md
│
├── cmd/
│   ├── zion-node/
│   └── zionctl/
│
├── internal/
│   ├── node/
│   ├── protocol/
│   ├── chain/
│   ├── consensus/
│   ├── p2p/
│   ├── identity/
│   ├── membership/
│   ├── governance/
│   ├── board/
│   ├── research/
│   ├── resources/
│   ├── objects/
│   ├── index/
│   ├── api/
│   ├── config/
│   └── resource/
│
├── apps/
│   └── web/
│
├── configs/
│   ├── alpha-1/
│   │   ├── genesis.*
│   │   ├── bootstrap.*
│   │   └── policy.*
│   └── local-dev/
│
├── schemas/
│   ├── object/
│   ├── chain/
│   ├── p2p/
│   └── api/
│
├── tests/
│   ├── integration/
│   ├── network/
│   ├── adversarial/
│   └── migration/
│
└── docs/
```

The exact layout MAY differ, but the following boundaries SHOULD remain explicit:

```text
protocol
chain
consensus
P2P
identity
governance
product domains
local index
API
UI
```

---

# 30. Minimum acceptance tests

A v0.1 build is not considered ready for external alpha testing until these minimum tests pass.

## 30.1 Protocol

- [ ] Canonical encoding produces stable bytes.
- [ ] Stable objects produce stable object IDs.
- [ ] Unsupported schema versions fail safely.
- [ ] Invalid signatures are rejected.
- [ ] Deterministic state replay produces identical state.

## 30.2 Identity

- [ ] Identity creation works.
- [ ] Signed objects verify correctly.
- [ ] Invalid signatures fail.
- [ ] Key rotation works.
- [ ] Revoked keys are rejected where required.

## 30.3 Chain

- [ ] Four validators reach deterministic finality.
- [ ] Loss of one validator within the supported fault model does not corrupt state.
- [ ] Invalid transactions are rejected.
- [ ] Restarted validators can synchronize canonical state.
- [ ] Validator-set changes are auditable.

## 30.4 Networking

- [ ] Three or more nodes discover each other.
- [ ] A first-boot node can join through configured or DNS bootstrap peers.
- [ ] A restarted node can reconnect using its persisted peer cache.
- [ ] A node can join through a manually supplied peer address.
- [ ] Existing peers continue operating if one bootstrap peer stops.
- [ ] A NORMAL node behind NAT can participate by initiating outbound connections to reachable peers.
- [ ] Inbound reachability is not required for ordinary NORMAL-node use.
- [ ] Wrong network IDs are rejected.
- [ ] Unexpected bootstrap peer identities are rejected.
- [ ] Incompatible protocol versions fail safely.
- [ ] P2P queues remain bounded.

## 30.5 Board

- [ ] Member A publishes a signed post.
- [ ] Member B receives it without a central database.
- [ ] Member C can synchronize it after being offline.
- [ ] Authorship remains verifiable.
- [ ] Invalid signed posts are rejected.

## 30.6 Research

- [ ] A paper proposal can be created.
- [ ] Governance can accept or reject it.
- [ ] Accepted metadata becomes canonical registry state.
- [ ] Duplicate detection prevents obvious duplicate admissions.

## 30.7 Resources

- [ ] A repository/tool/dataset reference can be proposed.
- [ ] Governance can accept or reject it.
- [ ] Accepted metadata becomes canonical Resource Registry state.
- [ ] No repository code is executed automatically.

## 30.8 Governance

- [ ] Only eligible ACTIVE members vote.
- [ ] One member receives one vote.
- [ ] Minimum unique voter threshold is enforced.
- [ ] Approval threshold is enforced.
- [ ] Vote replay/double-counting is rejected.

## 30.9 Resource limits and efficiency

- [ ] Oversized messages are rejected.
- [ ] Oversized objects are rejected.
- [ ] Per-peer rate limits work.
- [ ] Memory-sensitive queues remain bounded under hostile traffic.
- [ ] Storage does not exceed configured limits.
- [ ] Free-disk reserve is respected.
- [ ] Idle nodes do not continuously consume excessive CPU.
- [ ] A NORMAL node remains stable in a 1 GiB-class test environment under ordinary alpha load.
- [ ] Validator memory/CPU usage is measured and documented.
- [ ] Resource usage remains bounded when peers repeatedly connect, disconnect, or submit rejected work.

## 30.10 API / Web

- [ ] Node API binds only to localhost by default.
- [ ] ZION Web operates against the local Node API.
- [ ] ZION Web does not require a production central database.
- [ ] Shutting down the web frontend does not stop the network.

## 30.11 Migration

- [ ] State export succeeds.
- [ ] Fresh nodes can import intended canonical state.
- [ ] Private keys are absent from public state export.

---

# 31. Product acceptance scenario

The final v0.1 acceptance scenario SHOULD demonstrate the product end-to-end.

Example:

```text
1. Start four validator nodes.
2. Start two normal nodes.
3. Create identities for Alice and Bob.
4. Admit both as ACTIVE alpha members.
5. Alice publishes a Board post about an AI-security paper.
6. Bob receives and replies to the post.
7. Alice proposes the paper for the Research Registry.
8. Governance votes.
9. The paper becomes a canonical Research Registry entry.
10. Bob proposes a related repository.
11. Governance votes.
12. The repository becomes a canonical Resource Registry entry.
13. Board posts reference the paper and repository.
14. Shut down ZION Web.
15. The nodes and chain continue operating.
16. Restart the web client and recover the same network state.
17. Export canonical state.
18. Import it into a fresh local test environment.
```

If this scenario works reliably, v0.1 has demonstrated the ZION product concept.

---

# 32. Pre-genesis freeze checklist

Before signing the `zion-alpha-1` genesis, review:

- [ ] Network ID
- [ ] Genesis hash
- [ ] Initial validator identities
- [ ] Validator quorum/fault assumptions
- [ ] Membership admission rules
- [ ] Canonical serialization rules
- [ ] Object-ID format
- [ ] Hash/signature algorithm identifiers
- [ ] Key-rotation format
- [ ] Protocol version negotiation
- [ ] Maximum object/message sizes
- [ ] Resource limits
- [ ] Governance voter eligibility
- [ ] Governance minimum voter count
- [ ] Governance approval threshold
- [ ] Research Registry schema
- [ ] Resource Registry schema
- [ ] State export/import format
- [ ] Bootstrap peer replacement mechanism
- [ ] Bootstrap peer identity verification
- [ ] Peer-cache persistence behavior
- [ ] DNS/static/manual discovery fallback behavior
- [ ] NORMAL / VALIDATOR / BOOTSTRAP role semantics
- [ ] NAT outbound-participation behavior
- [ ] Resource-efficiency benchmark targets
- [ ] API bind defaults
- [ ] Alpha reset/migration warning

---

# 33. Deferred roadmap

The following roadmap is non-normative and may change.

## v0.2 — Distributed Storage

Potential focus:

```text
Chunking
Large-object storage
Deduplication
Replication
Pinning
Unpinning
Garbage collection
Paper mirroring
Repository snapshots
```

## v0.3 — Community Economy

Potential focus:

```text
ZION Points
Contribution provenance
Proposal deposits
Slashing policy
Reputation-aware governance
```

## v0.4 — Resilient Connectivity

Potential focus:

```text
AutoNAT
Hole punching
DCUtR
Community relay network
Advanced bootstrap/relay resilience
```

## v0.5 — Resource Preservation

Potential focus:

```text
Automatic upstream watchers
Safe repository ingestion
Immutable snapshots
Distributed CDN
Artifact delivery
Supply-chain verification
```

## v0.6 — Knowledge Network

Potential focus:

```text
Canonical knowledge graph
Extensible relation namespaces
Community vs canonical edges
Research/resource relationships
Graph governance
```

## Later

Potential future work:

```text
ZION Desktop
Private groups / native E2EE
Proof-of-Human
Hardware attestation
Advanced consensus hardening
Formal protocol verification
Adversarial-agent simulation
Private overlay networking
Advanced metadata-resistant networking
Any future transferable economy only if justified
```

---

# 34. Final implementation principles

The first principle of v0.1 is:

```text
Build enough decentralization to prove the ZION product,
not every mechanism required by the long-term vision.
```

The second principle is:

```text
Do not take shortcuts in protocol foundations
that become expensive to replace later.
```

Therefore v0.1 deliberately keeps:

```text
canonical encoding
object identity
identity/signatures
deterministic state transitions
protocol versioning
resource bounds
chain/content/UI separation
state migration
```

while deferring broader infrastructure.

The third principle is:

```text
ZION v0.1 is successful when humans can actually use it.
```

A working Board, Research Registry, Resource Registry, governance flow, and web interface across several independent nodes are more important than implementing every future subsystem before the product is visible.

The fourth principle is:

```text
A normal ZION node should remain practical on inexpensive commodity hardware.
```

Resource efficiency, bounded queues, bounded storage, and graceful rejection under hostile load are part of the security model, not merely cost optimizations.

---

# 35. Definition of done

`zion-alpha-1` v0.1 — Minimum Decentralized Product is done when:

```text
Several independent Go ZION Nodes can form a network,
reach deterministic chain finality,
manage cryptographic identities and membership,
allow normal nodes behind NAT to participate through outbound peer connections,
exchange signed Board content,
curate Research and Resource Registry entries through simple governance,
expose the network through a usable Next.js interface,
operate without a mandatory central application database,
remain practical on inexpensive commodity hardware under ordinary alpha load,
remain bounded under basic hostile-resource tests,
and export/import canonical state for alpha migration.
```

Everything beyond this definition is optional for v0.1 and SHOULD NOT block the first usable product.
