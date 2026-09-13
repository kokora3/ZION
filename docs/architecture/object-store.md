# Small Content-Addressed Object Store

Phase 9 adds bounded, immutable object storage and direct retrieval without putting object bytes or availability into consensus state. It reuses the frozen Phase 2 `ObjectID` derivation exactly; Phase 9 does not introduce a second identifier.

## Object and identifier model

A stored object is canonical CBOR containing the existing `UnsignedObjectCore` and its opaque payload. The core declares the object type, schema, protocol timestamp, content hash, payload size, visibility, and bounded metadata. Validation requires the declared size and SHA-256 content hash to match the payload. `ObjectID` remains the SHA-256 digest of the canonical Phase 2 core, excludes the resulting ID itself, and is distinct from the payload `ContentHash`.

```text
UnsignedObjectCore + payload
          |
          +-- validate size and ContentHash
          +-- canonical CBOR (maximum 1 MiB)
          |
          v
Phase 2 ObjectID
          |
          v
sha256/aa/bb/<64-lowercase-hex>.obj
```

Only validated algorithm and digest components determine a storage path. User filenames, API paths, and unvalidated identifier strings never do. Writes use a same-directory owner-only temporary file, flush it, and atomically rename it into the digest-sharded destination. Duplicate puts verify the existing bytes and return `ALREADY_EXISTS`; they never overwrite corrupt content. Reads re-decode canonical CBOR and recompute the identifier, so truncation, mutation, or misplaced content is reported as corruption.

The zion-alpha-1 hard limit is 1 MiB for the complete canonical stored representation. The default local quota is 1 GiB and is configurable per node. Quota accounts for actual canonical bytes. Full stores reject new objects without evicting existing content; existing reads remain available. There is no garbage collection, pinning, eviction, chunking, manifest, replication policy, or availability promise in Phase 9.

## Direct P2P retrieval

The versioned `/zion/object/0.1.0` protocol runs on the authenticated Phase 7 libp2p host. A bounded request contains schema version, NetworkID, and strict ObjectID. A bounded response is `FOUND`, `NOT_FOUND`, `TOO_LARGE`, or `STORE_ERROR`; only `FOUND` carries canonical object bytes. Both messages use canonical CBOR and reject non-canonical, malformed, oversized, wrong-network, or mismatched-ID input.

The requester checks local storage first, selects only usable authenticated ZION peers, deduplicates and bounds candidates, applies per-peer and overall timeouts, and verifies returned canonical bytes, declared content hash, and ObjectID before storing. PEX is only discovery: it does not make object data trustworthy. Concurrent requests for the same ObjectID are coalesced, inbound and outbound work is bounded, and cancellation shuts down streams and handlers.

This is direct best-effort retrieval, not a DHT or content-routing network. A peer may omit, corrupt, or refuse content. A valid object is content-authentic with respect to its identifier, not necessarily authored, endorsed, safe, available, or permanent.

## Local API and CLI

The versioned local API accepts and returns base64 wrappers around exact canonical object bytes:

- `POST /v1/objects` validates and stores one object.
- `GET /v1/objects/{object_id}` reads local storage only.
- `GET /v1/objects/{object_id}/meta` exposes verified public metadata, never a filesystem path.
- `POST /v1/objects/{object_id}/fetch` returns `ALREADY_LOCAL` or performs bounded P2P retrieval and returns `FETCHED`.

The API never accepts a server-side filename. `zionctl object put` reads a local canonical object file and sends bytes through the API. `object get` validates the returned bytes and writes locally, refusing overwrite unless `--force`; `stat` and `fetch` also use the API only. Existing loopback defaults, bearer authentication for non-loopback binding, exact-origin CORS, timeouts, and concurrency limits apply.

Runtime status reports `object_store_enabled`, `object_count`, `object_bytes`, and `object_quota_bytes`. It deliberately omits the storage path. Object directory, quota, file modification time, peer choice, fetch failures, and local availability are operational state.

## Consensus isolation and threat boundary

```text
Canonical chain state  ---> StateHash ---> AppHash

Local object files ----X
Object quota ----------X
Peer availability -----X
Fetch order/timing -----X
```

Storing, fetching, deleting outside the implementation, corrupting, or failing to retrieve an object does not mutate membership, governance, validators, transactions, `StateHash`, or `AppHash`. References may be canonical in later phases, but availability remains off-chain. The current `LOCAL` visibility label is metadata, not encryption or access control; Phase 9 object transport and storage provide no confidentiality.

Phase 10 builds signed Board events on top of this unchanged generic store. Research Registry, Resource Registry, chunking, erasure coding, automated replication, garbage collection, DHT/provider records, and permanent storage guarantees remain deferred.
