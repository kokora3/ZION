# Install ZION on Linux

Verify `zion-v0.1.0-alpha.1-linux-amd64.tar.gz` against trusted `SHA256SUMS` with `sha256sum -c SHA256SUMS`, extract the archive, and run `./run-zion-node.sh`. No Go toolchain or root access is required for the portable run. Status is available through `./zionctl status`.

The helper resolves its own directory, creates `data/normal`, and preserves all files across restarts. UDP 42000 is general QUIC/libp2p. TCP 42001 is the loopback API and metrics listener. Replace the bundled local-only GenesisID and bootstrap list before joining shared infrastructure.

For a system service, install the binaries under `/usr/local/bin`, create a dedicated `zion` user, place a reviewed configuration at `/etc/zion/zion.yaml`, create `/var/lib/zion` owned by that user, and adapt `deploy/systemd/zion-node.service`. The example is not installed automatically and contains no secrets.

Upgrade by stopping cleanly, exporting and backing up state plus the full data directory, replacing binaries, and restarting with the same data. Verify version, NetworkID, PeerID, height, and StateHash. Never share a mutable data directory or reset it as an upgrade shortcut.

