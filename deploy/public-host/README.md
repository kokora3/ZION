# Generic public Linux host

This directory prepares one `zion-alpha-1` **NORMAL + BOOTSTRAP** node. It overlays the canonical D1 `docker-compose.yml`; it does not fork the node image, provision a host, change DNS/firewalls, or deploy ZION Web.

```bash
cp deploy/public-host/.env.example deploy/public-host/.env
# Edit ZION_PUBLIC_HOST and ZION_PUBLIC_HOST_KIND; keep an explicit image tag.
sudo bash deploy/public-host/deploy.sh --check
sudo bash deploy/public-host/deploy.sh
bash deploy/public-host/status.sh
```

Release mode pulls the pinned D1 image in `ZION_IMAGE`. Until that image is deliberately published, use `ZION_DEPLOY_MODE=local` to build the exact D1 Dockerfile into the same configured image reference. Never use `latest`.

The override publishes only the configured host UDP port to container UDP 42000. API and metrics TCP 42001 have no host publication. Config is mounted read-only at `/etc/zion/zion.yaml`; all replaceable-container state, including the PeerID, remains under `/var/lib/zion` on the configured host bind mount.

See [the full public Internet node guide](../../docs/deployment/public-internet-node.md) and [the operator checklist](PUBLIC-HOST-CHECKLIST.md).
