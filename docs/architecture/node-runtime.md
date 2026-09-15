# Unified Node Runtime

Phase 8 composes the existing chain, governance, CometBFT adapter, general libp2p host, durable application snapshot, state sync, transaction relay, and local API. It does not duplicate canonical transition logic.

## Lifecycle and roles

The explicit lifecycle is CREATED, STARTING, RUNNING, STOPPING, STOPPED, or FAILED. Start is single-use, Stop is idempotent, all background work owns a cancellable context, and a partial startup failure closes every component already opened.

Startup order is configuration validation, state-envelope verification or explicit fresh initialization, optional consensus-application construction against that verified state, initial snapshot persistence, libp2p, Phase 8 stream handlers, optional CometBFT engine start, API, then discovery/sync. Shutdown cancels workers and closes API, consensus, and libp2p in reverse dependency order.

NORMAL runs state, libp2p, synchronization, relay, and API. VALIDATOR additionally runs the production CometBFT Service; advertised/configured capability is reported separately from authorization by the canonical or genesis validator key set. BOOTSTRAP adds no authority and uses the Phase 7 PEX behavior. Capabilities may overlap.

The strict YAML loader requires `consensus.enabled: true` and the `VALIDATOR` role together. Its `root_directory` points at an already provisioned CometBFT genesis, private-validator key/state, node key, and engine database. The runtime verifies the public genesis fingerprint and validator public key, then constructs the ABCI application only after loading the matching durable ZION snapshot. It never generates or conflates validator, member, and libp2p keys. General-P2P and CometBFT listen addresses are configured independently.

## Durable state

The local state/application.snapshot file is a version-1 JSON storage envelope containing NetworkID, GenesisID, accepted height, expected StateHash, and the exact canonical CBOR chain Snapshot bytes. Storage schema and canonical state schema are separate. Loading is bounded, rejects unknown/trailing data, verifies network/genesis, reconstructs State through StateFromSnapshot, requires exact canonical round-trip bytes, and recomputes StateHash.

Writes use a synced temporary file and a previous-file backup during replacement. A failed install attempts to restore the last snapshot. Windows replacement has a short rename window and is not claimed to have stronger durability than the filesystem provides. Corruption never causes an automatic genesis reset. CometBFT keeps its own database under its own directory.

## State synchronization

Authenticated usable peers serve bounded canonical offers on /zion/state/0.1.0. An offer binds network, Phase 5 GenesisID, accepted height, StateHash, and at most 8 MiB of canonical snapshot bytes. A normal node validates every field and the reconstructed hash before persistence and in-memory promotion. Lower-height rollback and equal-height conflict fail explicitly; failed validation leaves current state unchanged. Live validator state is never replaced through this path.

This is alpha snapshot synchronization, not a light client. Authentication, network/genesis binding, canonical bytes, and hash integrity do not prove Byzantine finality. The receiver compares valid available peers and rejects same-height conflicts, but operators should use trusted/checkpointed finalized metadata until a real light-client proof is implemented.

Non-validator nodes retry synchronization with capped backoff. General P2P independently maintains outbound peers for the full runtime lifetime, so synchronization can recover after a configured bootstrap restarts without restarting the local node. Successful synchronization is periodically refreshed so a relayed transaction can later become visible as committed state. Snapshot offers may include at most 256 bounded recent finality notices; these are operational status hints tied to the accepted offer and never replace canonical validation.

## Transaction relay and finality

/zion/tx/0.1.0 carries the existing canonical signed transaction bytes. The 65,536-byte protocol limit, strict consensus decoder, NetworkID, TxID, and non-mutating local state validation run before submission. A bounded recent-TxID cache prevents loops; relay is one hop and concurrency/time bounded. Relay or CheckTx acceptance is not finality. Only a committed observer or accepted synchronized committed snapshot produces FINALIZED status.

## Data directory and boundaries

    p2p/peer.key              independent P2P secret
    p2p/peers.json            local peer cache
    state/application.snapshot hash-verified canonical application state
    consensus/                CometBFT-owned keys and database

Operational lifecycle, peer cache, heights, sync status, API configuration, and recent transaction status are not canonical state. Member, P2P, and validator keys remain separate. Phase 9 object storage, Board/registry synchronization, full block sync, DHT, relay, hole punching, and the web UI are not implemented here.

## Alpha operational bounds

Canonical transactions remain limited to 65,536 bytes. Snapshot payloads are limited to 8 MiB, finality hints to 256, recent transaction status to 1,024 entries, concurrent relay handlers to 16, and concurrent state-serving handlers to 4. The API defaults to a 96 KiB body and 32 concurrent requests. Normal nodes use a 10-second sync attempt, a five-second successful refresh interval, and failure retry capped at one minute. Graceful shutdown defaults to ten seconds. These are local operational controls and do not affect StateHash.
