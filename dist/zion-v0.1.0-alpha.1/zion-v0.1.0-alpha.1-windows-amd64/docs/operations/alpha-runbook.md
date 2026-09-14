# zion-alpha-1 Runbook

ZION `v0.1.0-alpha.1` is experimental. History, genesis, validator membership, and schemas may reset before a later stable network. Never use test fixture keys.

## Start a node

1. Install the matching `zion-node` and `zionctl` archive and verify it against `SHA256SUMS` through a trusted channel.
2. Copy the matching profile from `configs/alpha-1`. Replace the GenesisID with the value from the shared validator genesis, choose a unique data directory, and add authenticated `/p2p/<PeerID>` bootstrap addresses.
3. Keep member signing, libp2p, CometBFT validator, and API bearer secrets separate. Restrict their files to the operating-system account where supported.
4. Keep the local API at `127.0.0.1:42001` unless explicit bearer authentication, firewalling, and TLS termination are configured.
5. Run `zion-node run --config zion.yaml`. Check `zionctl status`, `zionctl peers`, `GET /v1/health`, and `GET /metrics`.

NORMAL nodes do not run CometBFT. The outbound-only profile opens no general-P2P listener and joins through outbound authenticated connections. BOOTSTRAP is a replaceable discovery role, not authority. Validators must use separate CometBFT data, keys, and ports; their public validator key must match both the shared genesis and canonical governance set.

## Ports

| Service | Example | Default exposure |
|---|---:|---|
| ZION libp2p QUIC v1 | UDP 42000 | peer-facing when listening |
| Local ZION API and `/metrics` | TCP 42001 | loopback only |
| ZION Web | TCP 3000 | loopback development UI |
| CometBFT P2P | TCP 26656 | validator peers only |
| CometBFT RPC | disabled | do not expose by default |

Every node/process needs unique bound ports and a separate data directory.

## Alpha reset

Before an announced reset, stop mutations, export canonical state, record its StateHash/height, and back up keys and desired off-chain objects separately. An export is evidence/recovery material, not a promise that it can be imported across an incompatible genesis or future schema. Operators must explicitly install new configuration and follow published migration instructions.
