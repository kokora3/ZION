# Signed Community Board

Phase 10 provides a small public discussion substrate for ZION. An ACTIVE member signs a POST or REPLY event with the member's current Ed25519 key. The event and its content are immutable Phase 9 objects; ordinary Board activity is off-chain and requires no transaction, governance vote, validator privilege, or central database.

```text
ACTIVE Identity
      |
      v
Signed Board Event
      |
      +--> content ObjectID --> Object Store
      |
      +--> P2P announce/sync
      |
      v
Local Derived Index
      |
      +--> Feed
      +--> Thread
      +--> Search
```

## Objects, identifiers, and signatures

Content uses the versioned `zion.board/content/v1` object type. Its canonical CBOR payload declares schema 1, TEXT or MARKDOWN, an optional UTF-8 title, and a non-empty UTF-8 body. A signed event uses `zion.board/event/v1` and includes schema, NetworkID, POST/REPLY kind, author IdentityID and KeyID, protocol timestamp, optional parent PostID, content ObjectID, and bounded community references. The separate network-bound signing purpose is `zion.board.event/v1`; changing the kind, author, timestamp, parent, content, or any reference invalidates the signature.

The complete canonical signed-event object is hashed with the unchanged Phase 2 derivation. That ObjectID is the `post_id`. It commits to the signature as well as the event fields. The event's `content_object` is a different ObjectID, pointing to independently validated content. A reply must reference a valid parent PostID; locally authored replies require that parent to be present.

Community references are signed off-chain relationships with bounded `relation`, `target_kind`, and `target_id`. An OBJECT target must be a strict ObjectID. A reference is an author's statement, not canonical endorsement or governance authority.

Local publication requires an identity known to current chain state, ACTIVE membership, the currently active KeyID, a valid Board-domain signature, locally available valid content, and—when replying—a known parent. PENDING, SUSPENDED, REVOKED, unknown, and retired-key authors cannot create new local publications. A previously valid event can remain cryptographically verifiable after rotation or suspension. Remote admission therefore reports `SIGNATURE_VALID`, current membership, and either `CURRENTLY_AUTHORIZED` or `HISTORICAL_OR_UNCONFIRMED` separately; this is an honest current-state classification, not proof that a remote node observed the exact chain state at the event timestamp.

## Bounds

- Content body: 65,536 UTF-8 bytes; title: 256 bytes.
- Signed event payload: 32 KiB; the complete event object is hard-limited to 34 KiB.
- References per event: 32; relation: 96 bytes; target kind: 64 bytes; target ID: 256 bytes.
- Announcement: one bounded event object. Sync request: 1 KiB. Sync response: 1 MiB and at most 24 events.
- Default operational fanout is 8, sync page size 16, cycle maximum 256, concurrent handlers 8, peer timeout 3 seconds, and periodic anti-entropy interval 30 seconds. Configuration may tune defaults only within compiled hard limits.
- Local feed, reply, and search pages are limited to 100 results; search input is at most 512 UTF-8 bytes.

These controls reduce resource-exhaustion risk. They do not provide perfect spam resistance or proof-of-human.

## Announcement, anti-entropy, and content pull

`/zion/board/announce/0.1.0` carries one canonical event object to a bounded sample of authenticated usable ZION peers. `/zion/board/sync/0.1.0` pages event objects in lexical PostID order with a strict cursor and maximum page size. Periodic anti-entropy repairs events missed while a peer was offline. Announce origin and sync peers are untrusted: each event is decoded canonically, its ObjectID and Board signature are checked, and its author is classified against local chain state before storage responsibility is accepted.

After event admission, content is read locally or pulled through the existing bounded Phase 9 object protocol. Content ObjectID and Board schema are independently verified. Missing content is represented explicitly and may be repaired later. P2P announcements do not imply consensus acceptance, permanent replication, or a promise that content is available.

Because synchronization uses ordinary authenticated peers, discovered peers can continue directly after bootstrap disappears. An outbound-only NORMAL node can announce, sync, and pull content through connections it initiates; Board adds no inbound reachability requirement.

## Derived index, queries, and local moderation

The JSON Board index contains derived post/thread/search metadata and local visibility choices. It is not authoritative: the node can rebuild event rows from verified immutable Board objects, then resolve content already present in the object store. A corrupt or missing index cannot mutate chain state. Feed order is newest protocol timestamp first with PostID as a deterministic tie-breaker. Reply order is oldest first with PostID as the tie-breaker. Search is bounded Unicode-aware case-insensitive substring matching across title, body, author, and community references.

Hide/unhide is local moderation metadata. A hidden item is omitted from default local feed/search/thread results, but its event and content objects are not deleted and other peers can retain or redistribute them. There is no global deletion, central moderator, or remote takedown authority.

## Chain isolation and public-content warning

```text
Board authenticity != chain consensus
Board availability   != StateHash

Board objects/index/visibility ----X----> canonical StateHash/AppHash
```

Board signatures establish event authorship relative to a key; they do not grant membership, validator status, governance votes, or chain authority. Board objects, index files, peer timing, local hiding, and content availability never enter canonical snapshots. Public Board content may persist on independent peers after a local node hides or loses it. Phase 10 provides no private groups, encryption, anonymity, guaranteed deletion, content safety, or permanent availability.

Phase 11 may define a Research Registry. Phase 10 does not implement research/resource registries, a canonical graph, large-file chunking, automated replication, garbage collection, DHT content routing, or a product web UI.
