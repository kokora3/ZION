# ZION portable node for Linux

This amd64 alpha package needs no Go toolchain.

1. Verify the outer tarball against the trusted `SHA256SUMS`.
2. Extract it and run `./run-zion-node.sh`.
3. In another shell, run `./zionctl status`.
4. Stop with Ctrl+C. Data remains under `data/normal`.

The bundled config is a local-only preset. Replace its GenesisID and bootstrap peers with verified values before connecting to a shared deployment. UDP 42000 is the general libp2p listener; TCP 42001 is loopback-only API/metrics. Never share one mutable data directory between processes.

See `docs/guides/install-linux.md` and the optional systemd unit for server operation.

