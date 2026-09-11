# Canonical Encoding

ZION v0.1 uses RFC 8949 Core Deterministic CBOR: preferred integers and lengths, no indefinite items, and deterministic map ordering. `UnsignedObjectCore.Metadata` text is NFC-normalized; opaque payload bytes are never normalized. Timestamps are signed Unix milliseconds UTC. Only schema 1 is accepted; malformed CBOR, unknown fields, duplicate keys, tags, invalid UTF-8, and unsupported schemas fail safely.

`HashDigest` is self-described SHA-256. `ContentHash` hashes payload bytes; `ObjectID` hashes canonical unsigned body bytes and is `zion:obj:sha256:<lowercase-hex-digest>`. A body excludes its derived ID, preventing self-hashing. Golden vectors are fixed fixtures under `internal/protocol/testdata`.
