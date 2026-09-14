# ZION portable node for Windows

This unsigned alpha package needs no Go, Node.js, Git, Docker, or Visual Studio.

1. Verify the outer ZIP against the published `SHA256SUMS` obtained through a trusted channel.
2. Extract the entire directory.
3. Double-click `run-zion-node.cmd`, or run it from PowerShell.
4. In another PowerShell window, run `.\zionctl.exe status`.
5. Stop with Ctrl+C. The node reuses `data\normal` on restart.

The bundled `configs\normal.yaml` uses a local-only alpha genesis fingerprint and no bootstrap peer. Replace those fields with verified operator values before connecting to a shared deployment. Windows may ask whether UDP 42000 should be allowed through the firewall; allow only the network scopes you intend. The API remains on loopback TCP 42001.

Advanced start:

```powershell
.\zion-node.exe run --config .\configs\normal.yaml
```

Never run two nodes against the same data directory. Back up data before replacing binaries; do not delete it during an upgrade.

