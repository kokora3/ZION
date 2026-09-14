# ZION v0.1 Threat Model Summary

## Assets and authorities

Canonical state and finality are protected by deterministic validation and greater-than-two-thirds CometBFT voting. Member keys authorize identity/governance/Board statements; validator keys authorize consensus votes; P2P keys authenticate transport. No one key substitutes for another.

## Adversaries considered

Malformed network/API/file inputs, wrong-network peers, malicious bootstrap/PEX hints, duplicate/replayed transactions, retired keys, unauthorized members, corrupt snapshots/objects/indexes, bounded traffic floods, misleading registry metadata, untrusted Board markup, and a lost minority of validators are expected to fail without gaining authority or partially mutating canonical state.

## Explicit limitations

ZION v0.1 does not provide proof-of-human, Sybil resistance for transport peers, anonymity, traffic-analysis resistance, private groups, E2EE, light-client verification, automatic object replication/permanence, global Board deletion, executable-resource safety, guardian key recovery, hardware-backed storage, secure auto-update, or protection when more than one-third of validator power is Byzantine. Local host compromise can expose local keys and bearer tokens. DNS/bootstrap compromise can deny or misdirect discovery but cannot impersonate the expected PeerID without its key.

State sync authenticates a peer and verifies hashes but does not prove BFT finality independently. Resource and Research admission is curation, not a malware or truth guarantee. Operators must sandbox anything they obtain separately; ZION never executes registered content.
