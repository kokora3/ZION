# General P2P, Bootstrap Discovery, and Peer Resilience

ZION Phase 7 uses go-libp2p v0.49.0 (MIT license) for general node connectivity. It is not a replacement for CometBFT. CometBFT continues to own validator consensus transport and BFT voting; libp2p supplies encrypted, authenticated general-node transport, discovery, bounded peer exchange, and the substrate for future node services.

```text
                   ZION Node
                       |
        +--------------+--------------+
        |                             |
        v                             v
  CometBFT Network              ZION General P2P
  validator consensus           libp2p
        |                             |
        |                             +-- bootstrap/discovery
        |                             +-- peer exchange
        |                             +-- future node services
        |
        +-- BFT only
```

## Three identity domains

Member `IdentityID`, CometBFT validator identity, and libp2p `PeerID` are separate cryptographic domains with independent private keys. Member identity authorizes authorship, membership, and governance. The validator key authorizes BFT votes. The P2P key authenticates transport connections and derives the PeerID. Replacing a P2P key creates a new PeerID but does not alter IdentityID, membership, validator identity, governance rights, canonical snapshot bytes, `StateHash`, or `AppHash`.

```text
PeerID = WHO
DNS/IP = WHERE
```

The persisted P2P key is generated with a cryptographic random source and stored outside canonical state. The implementation requests owner-only directory and file modes. POSIX systems enforce those modes in the usual way; Windows does not map mode bits to a complete per-user ACL, so the containing node-data directory must also be protected by the operator's Windows account permissions. Hardware-backed storage is deferred.

## Transport and ZION hello

Phase 7 explicitly enables libp2p QUIC v1 and disables relay. QUIC supplies encrypted, authenticated transport and binds the remote public key to the libp2p PeerID. ZION does not add custom transport cryptography. TCP fallback is not enabled in this phase.

After transport authentication, peers open `/zion/hello/0.1.0` and exchange canonical CBOR with a four-byte bounded length prefix. The message contains schema version, `NetworkID`, network fingerprint, a typed supported-version set, operational roles, the self-reported PeerID, and bounded advertised addresses. The reported PeerID must equal the PeerID authenticated by libp2p.

The network fingerprint is the existing Phase 5 `consensus.Genesis.GenesisID`: SHA-256 of the deterministic application-genesis JSON that binds NetworkID, initial StateHash, and validator-set hash. Phase 7 receives this public digest through local configuration; it does not define or hash a second genesis concept. Same NetworkID with a different digest is rejected.

Version negotiation intersects typed `(major, minor, patch)` values and selects the numerically highest common version. It never uses lexical string ordering and never silently crosses an incompatible major version. The current implementation supports 0.1.0.

Roles `NORMAL`, `VALIDATOR`, and `BOOTSTRAP` may overlap. They are public capability hints only. `VALIDATOR` does not prove membership in the canonical CometBFT validator set, and `BOOTSTRAP` grants no authority. `STORAGE` and `RELAY` are reserved but rejected in Phase 7.

## Discovery and local peer cache

```text
Fresh Node
   |
   v
Peer Cache
   |
   +-- success --> ZION peers
   |
   v
Bootstrap / DNS Seeds
   |
   +-- hello
   +-- PEX
   |
   v
Static Fallback
   |
   v
Manual Peer
```

Discovery uses that exact order. Each dial address must contain the expected `/p2p/<PeerID>`. IP and DNS multiaddrs are supported; DNS only locates the expected PeerID and cannot replace transport authentication. Multiple bootstrap entries are accepted, and failure of one does not block later entries.

The peer cache is versioned local JSON, not a wire or consensus format. It stores only bounded public contact records, source, last successful local time, and bounded failure/backoff data. Only a peer that completed libp2p authentication and the ZION hello becomes successful cache data. A corrupt or oversized cache yields an empty operational cache plus a diagnostic error and never reaches canonical state. Cache replacement uses a synced temporary file; POSIX rename is atomic, while Windows replacement requires removing the prior file and therefore has a small non-atomic window that is recovered as an empty cache.

Defaults are 64 connected ZION peers, 12 outbound targets, 8 concurrent dials, 256 cached peers, 8 addresses per peer, and 32 PEX records per response. Hard maxima are 256 connected peers, 32 concurrent dials, 1,024 cache entries, 16 addresses per peer, and 64 PEX records. Dial, hello, and PEX work uses contexts, deadlines, and capped exponential backoff.

## Bounded peer exchange

The `/zion/pex/0.1.0` protocol exchanges canonical CBOR hints. A response contains public PeerID, addresses, and known roles for a bounded sample of usable peers. It excludes self, requester, duplicates, peers without advertised reachable addresses, secrets, filesystem paths, and unrelated interfaces. PEX is untrusted discovery data: every candidate must still authenticate the claimed PeerID at the libp2p transport and pass network, genesis, version, role, and message validation.

Hello is limited to 16 KiB, eight versions, three roles, eight advertised addresses, and 512 bytes per multiaddr. PEX requests are limited to 1 KiB and responses to 64 KiB in addition to record/address counts. Non-canonical CBOR, unknown fields, indefinite lengths, invalid UTF-8, malformed PeerIDs, malformed multiaddrs, relay addresses, duplicate records, self records, and oversized frames fail without promotion to the usable peer set.

## Bootstrap non-authority and outbound-only nodes

A bootstrap node is an ordinary reachable introducer. It can authenticate connections and return PEX hints, but it cannot mutate chain state, approve membership, grant governance or validator power, or impersonate another PeerID. Once normal peers discover and connect directly, the bootstrap may disappear without breaking that connection. A restarted node can reconnect from its persisted peer key and successful cache without bootstrap availability.

A NORMAL node may use no listen addresses and initiate all connections outbound. It can dial bootstrap, complete hello, obtain PEX, and dial another reachable peer without public IP, port forwarding, UPnP, NAT-PMP, AutoNAT, hole punching, DCUtR, relay, or TURN.

## Threat and phase boundaries

A malicious bootstrap can omit peers, refuse service, or return junk hints. DNS can misdirect location. Neither can authenticate as the expected PeerID without its private key. This phase bounds ZION-owned input and retries but does not claim anonymity, Sybil resistance, universal inbound NAT connectivity, or precise production resource guarantees.

Phase 7 introduces no DHT, mDNS production dependency, chain/state synchronization, transaction or content gossip, central registry, application API, or runtime integration. Those service protocols and the complete node runtime remain Phase 8 or later work.
