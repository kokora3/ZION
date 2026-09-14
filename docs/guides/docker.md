# Run ZION with Docker

Docker packages the existing `zion-node`; it does not change protocol authority. Linux/amd64 is the supported D1 image platform. The node image also contains `zionctl`. The Web image is optional and has no central database or private-key custody.

## Profiles

```bash
docker compose --profile normal up -d
docker compose --profile bootstrap up -d
docker compose --profile validator up -d
docker compose --profile normal --profile web up -d
```

Use only one node profile at a time with the default host ports. NORMAL uses volume `zion-normal-data`; BOOTSTRAP uses `zion-bootstrap-data`; VALIDATOR uses `zion-validator-data`. Two containers must never write one volume. BOOTSTRAP remains a replaceable discovery service. VALIDATOR requires externally provisioned CometBFT genesis, private-validator key/state, and node key and remains authorized only when the verified genesis/canonical validator set says so. Set `ZION_VALIDATOR_CONFIG` to a reviewed config path if needed.

The shipped NORMAL/BOOTSTRAP Compose configs use the documented local-only GenesisID so a fresh isolated node can start. For shared operation, use the public genesis under `configs/alpha-1/`, whose frozen GenesisID is `72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6`, and add verified bootstrap peers. Do not treat the D1 Compose preset as a public genesis statement.

## Ports and API boundary

| Profile | Host publication |
|---|---|
| normal | UDP 42000; TCP 42001 bound to `127.0.0.1` |
| bootstrap | UDP 42000; TCP 42001 bound to `127.0.0.1` |
| validator | UDP 42000; TCP 26656; TCP 42001 bound to `127.0.0.1` |
| web | TCP 3000 bound to `127.0.0.1` |

Metrics share TCP 42001 and are not public by default. CometBFT RPC is disabled. The node listens inside its container on TCP 42001 and therefore requires a bearer token. The entrypoint generates a 256-bit token at first start under persistent `/var/lib/zion/runtime/api-token`; it never enters an image layer or Compose environment. Read it only when entering the token in local Web Settings. `NEXT_PUBLIC_ZION_API_URL` contains only the non-secret loopback URL.

## Operate and diagnose

```bash
docker compose ps
docker compose logs -f
docker compose exec zion-node-normal zionctl --bearer-token-file /var/lib/zion/runtime/api-token status
docker compose down
```

Substitute `zion-node-bootstrap` or `zion-node-validator` for the other profiles. Health checks call `zionctl status`; they verify authenticated status access. `/v1/health` still distinguishes process liveness from readiness. Containers use a non-root UID/GID 10001, drop all capabilities, set `no-new-privileges`, use a read-only root filesystem with bounded tmpfs, and allow 20 seconds for SIGTERM shutdown. A host bind mount must be writable by UID 10001; named volumes avoid the common ownership problem.

## Persistence, backup, and upgrade

Container replacement with the same named volume preserves the P2P identity, peer cache, canonical application snapshot, objects, Board/registry indexes, API token, and (for validators) CometBFT data. `docker compose down` retains named volumes; adding `--volumes` deletes them and is not an upgrade operation.

For an upgrade: stop; export canonical state and back up the named volume; pull or load the deliberately selected version tag; recreate with the same volume/config; then verify version, NetworkID, PeerID, StateHash, objects, and indexes. No D1 wrapper performs an implicit migration or reset.

For an Ubuntu VPS/DigitalOcean Droplet, install Docker Engine plus Compose, retain the named volume (or an owned bind mount), expose only peer ports in the firewall, and attach DNS/Reserved IP as appropriate. D1 does not deploy any provider resources.

## Build metadata and scanning

The node builder is `golang:1.27.1-alpine3.23`; runtime is `alpine:3.22`. Web build/runtime use `node:22.22.0-alpine3.22`. Images carry OCI version, revision, source, creation date, and license labels. CI builds and smoke-tests without publishing from pull requests. Run Trivy/Grype/Docker Scout locally if available; absence of a scanner is reported, never represented as a pass.
