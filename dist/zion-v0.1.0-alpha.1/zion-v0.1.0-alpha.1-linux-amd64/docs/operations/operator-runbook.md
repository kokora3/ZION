# Node Operator Runbook

## Routine checks

- `/v1/health`: `live=true`; `ready=true` only when the runtime is RUNNING and SYNCED.
- `zionctl status`: verify NetworkID, public GenesisID, height, StateHash, sync state, consensus authorization, object quota, and configured limits.
- `zionctl peers`: verify multiple authenticated peers rather than relying on one bootstrap.
- `/metrics`: alert locally on readiness loss, stale height, peer loss, object quota pressure, fetch failures, and API rejection changes. Do not publish the endpoint without the same API protections.
- JSON stderr logs: retain according to local policy; do not add secrets as wrapper-process attributes.

## Backup and recovery

Stop the node for a consistent operator backup. Back up the P2P key, CometBFT validator key/state (validators only), member keys (where separately managed), peer cache, desired immutable objects, and canonical export as separate classes. A canonical export intentionally excludes every secret and all off-chain availability/index state.

Create and inspect a state export:

```text
zion-node state export --config zion.yaml --out state-backup.json
zion-node state inspect --in state-backup.json
```

Import only into a stopped fresh NORMAL profile whose state path is empty:

```text
zion-node state import --config fresh-normal.yaml --in state-backup.json
```

Import refuses wrong network/genesis, corruption, unsupported schemas, recovery remnants, and existing state. It does not recover a validator's CometBFT database.

## Key loss

- P2P key loss creates a new PeerID. Replace cached/bootstrap addresses as needed; IdentityID, validator identity, and StateHash do not change.
- Validator key loss cannot be repaired by state import or a P2P/member key. Stop that validator, protect against double-signing, and use canonical governance to replace it where quorum permits. Restore private-validator state together with the key only from a trusted consistent backup.
- Member key loss has no server-side recovery. If an authorized rotation cannot be signed, v0.1 has no guardian recovery. Do not claim ownership based on PeerID or validator keys.

## Failures

A corrupt application snapshot fails closed; preserve it for diagnosis and restore only from a verified export/backup. A corrupt object is never overwritten silently. Object quota full leaves existing objects readable. Corrupt Board/registry indexes rebuild from authoritative objects/canonical state. Bootstrap loss should not break existing direct connections; retain multiple seeds and peer-cache backups. General state sync is trusted alpha bootstrap sync, not a light client.

For disk exhaustion, stop writes, free space outside the data directory, verify files, then restart. Do not delete CometBFT private-validator state or canonical snapshot files ad hoc. Graceful shutdown is preferred so temporary writes and caches finish cleanly.
