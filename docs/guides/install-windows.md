# Install ZION on Windows

Use the `zion-v0.1.0-alpha.1-windows-amd64.zip` portable package. Obtain `SHA256SUMS` through the same trusted release channel, run `Get-FileHash -Algorithm SHA256 <zip>`, and compare all 64 hexadecimal characters before extracting.

Run `run-zion-node.cmd`. It resolves its own directory, creates `data\normal`, and starts the bundled node with `configs\normal.yaml`. It never resets existing data. A Windows firewall prompt concerns the UDP 42000 peer listener; grant only intended network scopes. The API and metrics remain at `127.0.0.1:42001`.

From a second PowerShell window:

```powershell
.\zionctl.exe status
.\zionctl.exe peers
```

Advanced users can run `.\zion-node.exe run --config .\configs\normal.yaml`. Stop with Ctrl+C and allow graceful shutdown to finish. Restarting reuses the same P2P key, state, objects, and indexes under `data\normal`.

The bundled config is a self-contained local preset. Before joining a shared `zion-alpha-1` deployment, replace its local-only GenesisID and bootstrap addresses with verified operator values. Do not run two processes against one data directory.

For an alpha upgrade, stop the node, export canonical state, back up the complete data directory and configuration, replace only the binaries, then start with the retained data. Verify software version, NetworkID, PeerID, height, and StateHash. Never delete or silently migrate old state.

