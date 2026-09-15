# Persistent bootstrap reconnect: real D2B retest

Use binaries and the public-host image built from the same reviewed hotfix commit. Do not reuse the older D1 Windows archive, because it predates the `advertise_addresses` schema and reconnect worker.

1. Build and install the Windows `zion-node.exe` and `zionctl.exe` from the hotfix commit. Preserve the existing Windows data directory and P2P key.
2. Build and install the DigitalOcean `zion-node` image from that exact commit. Preserve `/opt/zion/data`.
3. Start the public NORMAL + BOOTSTRAP and the outbound-only Windows NORMAL.
4. Confirm both report NetworkID `zion-alpha-1`, GenesisID `72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6`, Windows PeerID `12D3KooWLTx9GPGJhx4YPyLquyMkk1qkLLvVsw8PcH6JdccqQJ61`, bootstrap PeerID `12D3KooWHK9tHQrJVLhkrsrwzxW9JUC4oFvZmgDg8bXw93imseKH`, and the same StateHash.
5. On Windows, confirm `peer_count=1`, `outbound_peer_count=1`, `sync_status=SYNCED`, and `zionctl peers` contains bootstrap PeerID `12D3KooWHK9tHQrJVLhkrsrwzxW9JUC4oFvZmgDg8bXw93imseKH` at `/dns4/bootstrap-zion.kokora.info/udp/42000/quic-v1`.
6. Leave the Windows process running. Record its PID and PeerID.
7. Manually reboot the DigitalOcean host. Do not restart Windows ZION.
8. Confirm the public container returns with the same PeerID, GenesisID, and StateHash.
9. Allow the bounded reconnect window. Retries start near two seconds, increase exponentially with jitter, and cap at five minutes; wait at least five minutes after the public node is healthy before declaring failure.
10. Confirm Windows returns to `peer_count=1`, `outbound_peer_count=1`, and `sync_status=SYNCED`; confirm `zionctl peers` contains the authenticated bootstrap PeerID.
11. Confirm the public node reports `peer_count=1` and that the Windows PID and PeerID never changed.

This manual real-host result is the final D2B acceptance evidence. The repository test suite does not reboot or modify the real provider host.
