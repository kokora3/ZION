# Canonical State Export and Recovery

`zion-node state export --config zion.yaml --out backup.json` first loads the durable application snapshot through the production verifier. It writes a new owner-only file and refuses an existing destination. The version-1 JSON envelope binds `network_id`, SHA-256 `genesis_hex`, canonical state schema, accepted height, exact canonical Snapshot CBOR bytes, and the corresponding `zion:state:sha256:` hash.

`zion-node state inspect --in backup.json` is bounded to 8 MiB plus a fixed envelope allowance. It rejects unknown/trailing JSON, invalid metadata, malformed or non-canonical CBOR, envelope/state disagreement, and recomputed StateHash mismatch.

`zion-node state import --config zion.yaml --in backup.json` is a deliberate recovery operation. Stop the node first. It accepts only a matching NetworkID/GenesisID and a destination with no application snapshot or recovery backup. CLI configuration containing a `VALIDATOR` role or enabled CometBFT is rejected. State schema V3 imports byte-for-byte; V2 uses the existing explicit deterministic registry bootstrap migration and reports `migrated: true`. V1 and unknown schemas are rejected because V1 lacks the governance bootstrap decision needed for safe automatic migration.

The export contains canonical public state only. Member/P2P/validator private keys, bearer tokens, peer cache, objects, Board local visibility, and derived indexes require separate operator backup where appropriate. Object and Board availability are deliberately not implied by a canonical export.
