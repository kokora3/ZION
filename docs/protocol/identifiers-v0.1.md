# ZION v0.1 Identifier Inventory

| Identifier | Namespace / derivation | Boundary |
|---|---|---|
| IdentityID | `zion:identity:sha256:<hex>` from identity genesis body | Stable canonical member identity across signing-key rotation |
| KeyID | `zion:key:sha256:<hex>` from public signing key | Canonical key identity; changes with key |
| PeerID | libp2p multiformat public-key identity | Local operational transport identity |
| Validator identity | CometBFT Ed25519 public key/address | Consensus voting identity; distinct from IdentityID and PeerID |
| TxID | `zion:tx:sha256:<hex>` from complete canonical transaction envelope | Canonical transaction identity; excludes itself |
| ProposalID | `zion:proposal:sha256:<hex>` from canonical proposal body | Canonical governance identity |
| ObjectID | `zion:obj:sha256:<hex>` from canonical unsigned object core | Content-addressed public object identity |
| ResearchID | `zion:research:sha256:<hex>` from canonical Research entry body | Canonical admitted registry identity |
| ResourceID | `zion:resource:sha256:<hex>` from canonical Resource entry body | Canonical admitted registry identity |
| StateHash | `zion:state:sha256:<hex>` from canonical state Snapshot | Canonical application commitment; digest maps to AppHash |

Typed parsers reject cross-namespace substitution. Software version, protocol version, state schema, wire schema, PeerID, DNS/IP location, and local filenames are not interchangeable identifiers.
