# ZION v0.1 P2P Protocol Inventory

All ZION-owned network messages use canonical bounded CBOR framing over an authenticated libp2p connection. These IDs are application protocols, not CometBFT consensus protocols.

| Protocol ID | Purpose | Main bounds/trust rule |
|---|---|---|
| `/zion/hello/0.1.0` | Network/genesis/version/role compatibility | authenticated PeerID must match; 16 KiB hello |
| `/zion/pex/0.1.0` | Candidate peer exchange | hints only; bounded peers/addresses |
| `/zion/tx/0.1.0` | Signed canonical transaction relay | 65,536-byte transaction plus fixed frame |
| `/zion/state/0.1.0` | Alpha snapshot sync | 8 MiB snapshot; verify network/genesis/canonical bytes/StateHash |
| `/zion/object/0.1.0` | Direct small-object retrieval | 1 MiB canonical object; recompute ObjectID/content hash |
| `/zion/board/announce/0.1.0` | Board event announcement | bounded ID hint; fetch and verify independently |
| `/zion/board/sync/0.1.0` | Bounded Board anti-entropy pages | signed events remain untrusted until verified |

The CometBFT transport is separate and owns BFT votes/finality. No P2P role claim, bootstrap response, or protocol stream grants membership, governance, or validator authority. The state-sync protocol is not a Byzantine light client.
