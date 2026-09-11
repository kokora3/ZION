# Identity Primitives

`IdentityID` derives from immutable genesis data (schema, initial public key, creation timestamp), so it remains constant across rotation. `KeyID` derives from a canonical public-key descriptor. Ed25519 signatures use canonical, domain-separated envelopes; network-scoped operations include `NetworkID`.

```text
Identity ID
 ├── Key A -- old-key rotation authorization
 └── Key B -- new-key possession proof
```

Cryptographic verification is not current authorization: chain state later determines active/retired/revoked keys. Membership is separately chain-governed (`PENDING`, `ACTIVE`, `SUSPENDED`, `REVOKED`); a valid identity or node is not automatically a member or validator. ZION member identities are not P2P Peer IDs.

## Phase 3 decoder surface

`IdentityGenesisBody`, `IdentityGenesisProof`, and `RotationProof` currently have no standalone public raw-byte decoder. They are constructed and validated through typed Go APIs. Adding production byte decoders solely for fuzzing would create an unnecessary boundary; later transaction/wire decoding must own and fuzz that boundary. Existing public attacker-controlled boundaries are KeyID and IdentityID parsing plus public-key and signature validation.
