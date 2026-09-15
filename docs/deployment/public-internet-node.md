# Provider-neutral public Internet node

## Scope and architecture

D2A prepares—but does not perform—the first Internet pilot deployment. One ordinary Linux host runs the existing D1 `zion-node` artifact with roles `NORMAL, BOOTSTRAP`. It is replaceable discovery/availability infrastructure, not a validator, governance member, canonical source of truth, relay, DHT node, Web host, or public API gateway.

```text
Windows browser -> local ZION Web -> local outbound NORMAL
                                           |
                                           | authenticated QUIC/UDP
                                           v
                              public NORMAL + BOOTSTRAP
```

The public bootstrap can disappear without changing protocol authority. Another compatible authenticated bootstrap may replace it. Provider mappings—such as a provider's stable-address or firewall product—belong to D2B and must never fork ZION's image, config semantics, PeerID logic, or GenesisID.

## Frozen network identity

The deployment consumes the G1 files under `configs/alpha-1/`:

```text
NetworkID: zion-alpha-1
GenesisID: 72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6
```

`deploy.sh` verifies those files through the `zionctl genesis verify` implementation included in the same D1 image, then requires the external bootstrap config to match the frozen identifiers. D1's local-only digest is rejected. No validator key, validator state, or CometBFT node key belongs on this host.

Keep the three identity concepts separate:

```text
DNS/public IP = replaceable network location
PeerID         = persistent libp2p cryptographic identity
GenesisID      = frozen chain/network-instance identity
```

Changing DNS/provider must not change PeerID or GenesisID. Losing `/var/lib/zion/p2p/peer.key` changes PeerID but not GenesisID.

## Host target and prerequisites

The first pilot target is Linux amd64, an Ubuntu LTS or comparable Docker-capable distribution, 1 GiB RAM, and a 25 GiB disk class with a stable public IPv4/IPv6 address. This is an alpha observation target, not a consensus guarantee or permanent production requirement; resize to 2 GiB if measurement shows memory pressure. A modest 1–2 GiB host swap file may protect a small pilot from OOM, but is not a performance substitute and is never configured from the container.

Install Docker Engine and the Docker Compose plugin using Docker's official distribution-specific instructions. The primary workflow does not run opaque `curl | sh`, install Docker, create an account/VM/IP, or alter host services.

Recommended host layout:

```text
/opt/zion/config/bootstrap.yaml   public read-only config
/opt/zion/data/                   sensitive persistent node state
/opt/zion/backups/                sensitive operator backups
```

The runtime UID/GID is 10001. `deploy.sh` creates the exact configured directories and makes only data/backup writable by that UID. It refuses `/`, overlapping directories, an unpinned image, a changed reviewed config, invalid public location text, and invalid network/genesis configuration.

## Public and private ports

| Direction | Protocol | Port | Source | Purpose |
|---|---|---:|---|---|
| inbound | TCP | 22 | operator IP where practical | SSH/operator access |
| inbound | UDP | 42000 by default | Internet | ZION libp2p QUIC v1 |
| inbound | TCP 42001 | none | denied/not published | protected API and metrics |
| inbound | TCP 3000 | none | denied/not deployed | ZION Web |
| inbound | validator/RPC | none | denied/not deployed | no validator authority |
| outbound | required | provider-neutral | Internet | authenticated peer dialing and deliberate image pull |

The exact mapping is public host UDP `ZION_P2P_PORT` to container UDP 42000. The override deliberately removes D1's host-loopback API mapping; operators use `docker compose exec` through the helpers. Metrics share the protected container-local API listener and are not public. The override does not use host networking, privileged mode, added capabilities, host PID, or `NET_ADMIN`.

Configure a firewall manually after confirming SSH recovery. For UFW, a reviewed example is `sudo ufw allow from <operator-ip> to any port 22 proto tcp` followed by `sudo ufw allow 42000/udp`; inspect `sudo ufw status verbose` before enabling anything. For nftables, add an equivalent UDP destination-port accept rule only within the host's existing reviewed input table/chain. `deploy.sh` never changes firewall state, avoiding provider coupling and SSH lockout.

## Configure and validate

From a reviewed repository checkout:

```bash
cp deploy/public-host/.env.example deploy/public-host/.env
```

Edit `.env` without adding secrets. Set `ZION_PUBLIC_HOST_KIND` to `dns4`, `dns6`, `ip4`, or `ip6`; set `ZION_PUBLIC_HOST` to the stable location; keep an explicit image tag/digest and explicit P2P port. The helper derives:

```text
/<kind>/<public-host>/udp/<public-port>/quic-v1
```

The node binds `/ip4/0.0.0.0/udp/42000/quic-v1` inside Docker but advertises only the derived public address. Runtime status appends its real PeerID:

```text
/dns4/<hostname>/udp/<port>/quic-v1/p2p/<PeerID>
```

Release mode pulls `ghcr.io/kokora3/zion-node:v0.1.0-alpha.1` only after that pinned D1 image is deliberately published. D2A does not publish it. For local/CI validation set `ZION_DEPLOY_MODE=local`; this builds the exact D1 `deploy/docker/zion-node.Dockerfile`, not a public-host image.

```bash
sudo bash deploy/public-host/deploy.sh --check
sudo bash deploy/public-host/deploy.sh
```

`--check` verifies Linux, Docker/Compose, G1 public genesis, config, paths/permissions, the pinned image/build mode, exact Compose rendering, and explicit advertised multiaddr without starting the service. Deploy then starts only `zion-node-bootstrap`, waits for health, and prints status. It never installs Docker, changes firewall/DNS, creates cloud resources, resets state, deletes volumes/data, or generates validator material.

## Operate

```bash
bash deploy/public-host/status.sh
bash deploy/public-host/logs.sh
bash deploy/public-host/logs.sh --no-follow
sudo bash deploy/public-host/restart.sh
sudo bash deploy/public-host/stop.sh
```

Status returns software/build version, NetworkID, GenesisID, roles, PeerID, StateHash/height, readiness/sync, peer counts, object use/quota, actual bound P2P addresses, explicit advertised addresses, and copyable bootstrap multiaddrs. It never prints the peer private key or API token. Docker JSON logs rotate at 10 MiB with five files; `logs.sh` tails at most 5,000 lines and defaults to 200.

Stop is non-destructive. Restart records and compares PeerID and StateHash after health returns. `restart: unless-stopped`, the host's Docker boot configuration, and the same bind mount produce the expected host-reboot path; D2B must test the real reboot.

## Backup and update

Run a consistent backup while mutation is stopped:

```bash
sudo bash deploy/public-host/backup.sh
```

The helper stops a running node, writes a versioned secret-free canonical `state-export.json` using the actual `zion-node state export --config ... --out ...` syntax, archives the entire sensitive node data (including PeerID and API credential), records checksums, copies public config, then restarts if it was previously running. The node-data archive is sensitive: encrypt it, restrict it, never publish it, and keep it separate from public canonical exports. It contains no validator secret because this host is not a validator.

For an update, edit only `ZION_IMAGE` to another deliberate pinned D1 tag/digest, then run:

```bash
sudo bash deploy/public-host/update.sh
```

The helper records current identity/state, creates a backup while stopped, pulls or locally rebuilds the explicit D1 image, revalidates G1/config/Compose, recreates with the same data, waits for health, and requires unchanged NetworkID, GenesisID, PeerID, and StateHash. It never uses `latest`, silently migrates, or resets state. Preserve the backup and diagnose if validation fails.

## Join from a Windows NORMAL node

Copy one `bootstrap_multiaddrs` value from `status.sh` into the Windows NORMAL profile's `p2p.bootstrap_addresses`, using the same frozen GenesisID. Start the local Windows node, then inspect `zionctl status` and `zionctl peers`. Windows needs only outbound UDP connectivity; no inbound port forwarding, UPnP, relay, AutoNAT, DHT, or hole punching is required or introduced in v0.1.

The NORMAL node keeps that authenticated bootstrap address as a lifetime re-dial candidate. If the public host or container restarts, leave Windows ZION running: the outbound count falls, retries follow bounded exponential backoff with jitter, DNS is resolved on subsequent dials, and the same `/p2p/<PeerID>` must authenticate before the peer becomes usable again. Follow the real-host procedure in `docs/operations/persistent-bootstrap-reconnect.md` after installing binaries and images built from the same hotfix commit.

## Observe and troubleshoot

Record evidence for D2B/D3 with `docker stats`, `free -h`, `df -h`, `bash deploy/public-host/status.sh`, and bounded logs. Diagnose before changing data:

- No peers: verify the client GenesisID/network, full `/p2p/<PeerID>` multiaddr, DNS result, and bidirectional UDP/firewall path.
- Wrong GenesisID/NetworkID: compare with `configs/alpha-1/genesis-id.txt`; do not rewrite or reset data to conceal a mismatch.
- Private Docker IP advertised: set the reviewed `.env` public host/kind, rerun `deploy.sh --check`, and confirm `p2p_advertised_addresses` contains only the public location.
- Bad bootstrap address: require the current `quic-v1` form and runtime PeerID; an IP/DNS without PeerID is not authenticated bootstrap identity.
- UDP blocked/port conflict: inspect host and provider firewall plus `docker port`; free or explicitly change the configured external UDP port. The helper never silently selects another port.
- Unhealthy container: inspect bounded logs, then validate config and volume ownership; do not delete state.
- PeerID changed: stop deployment and restore the sensitive P2P identity backup to the same data path. DNS or provider changes do not explain a PeerID change.
- Volume permissions: make the exact data directory writable by UID/GID 10001; do not loosen unrelated host paths.
- OOM: inspect `docker stats` and kernel logs, add modest host swap only as protection, or resize to 2 GiB.
- Disk full: stop writes, free space outside ZION data, preserve snapshots/objects for diagnosis, then restart. Never begin with `rm -rf data`.

The local `test.sh`/distribution CI simulates NORMAL→BOOTSTRAP connection, explicit DNS advertisement, QUIC/UDP mapping, container recreation, wrong/local GenesisID, invalid config, occupied port, PeerID/StateHash persistence, provider-neutral runtime text, and secret boundaries without cloud credentials or live DNS.

## D2B boundary

D2B's first human action is to choose and provision one generic-spec Linux host, assign a stable public address, configure SSH and inbound UDP 42000 in the provider/host firewall, optionally create DNS, and make the pinned D1 image available through a trusted channel. Then run this unchanged deployment and complete every real-host item in `PUBLIC-HOST-CHECKLIST.md`, including reboot and Windows Internet connectivity. D2A creates none of those resources.
