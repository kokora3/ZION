# Distribution runtime modes

D1 distributes one `zion-node` implementation and selects capabilities through external configuration. NORMAL enables state, P2P, Board, objects, registries, sync, and API. BOOTSTRAP is NORMAL plus reachable Hello/PEX discovery. VALIDATOR is NORMAL plus CometBFT configured from externally mounted genesis and validator material.

A Compose profile is an operator preset, not canonical authority. BOOTSTRAP does not become a source of truth. VALIDATOR does not gain consensus membership because a profile or role string was selected: startup verifies the CometBFT genesis and validator public key, and effective authority still comes from the genesis/canonical validator set. Docker images contain neither authority nor secrets.

Portable binaries and containers use the same data layout concepts: P2P identity/cache, canonical application snapshot, objects, derived indexes, and optional CometBFT data are durable; configs are operator-controlled. Replacing a process or container while retaining the data directory must retain PeerID and application state.

For a public Docker bootstrap, the bind listener and advertised location are distinct. The container listens on its bridge interface while an explicit generic DNS/IP QUIC multiaddr replaces private container addresses in Hello/PEX and status output. This operational location changes neither PeerID nor GenesisID.
