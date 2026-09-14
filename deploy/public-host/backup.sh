#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/public-host/lib.sh
source "$SCRIPT_DIR/lib.sh"
load_settings
require_docker
verify_shared_identity

keep_stopped=false
[[ "${1:-}" != --keep-stopped ]] || keep_stopped=true
was_running="$(docker inspect --format '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null || true)"
if [[ "$was_running" == true ]]; then
  status_json >/dev/null
  compose stop "$SERVICE_NAME"
fi
restart_on_exit() {
  if [[ "$was_running" == true && "$keep_stopped" == false ]]; then
    compose start "$SERVICE_NAME" >/dev/null
    wait_for_health
  fi
}
trap restart_on_exit EXIT

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
target="$ZION_BACKUP_DIR/$stamp"
mkdir -p -- "$target"
if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
  chown 10001:10001 "$target"
else
  docker run --rm --user 0 --entrypoint chown -v "$target:/backup" "$ZION_IMAGE" 10001:10001 /backup
fi

docker run --rm --user 10001:10001 --entrypoint zion-node \
  -v "$ZION_DATA_DIR:/var/lib/zion" \
  -v "$ZION_CONFIG_DIR/bootstrap.yaml:/etc/zion/zion.yaml:ro" \
  -v "$target:/backup" "$ZION_IMAGE" \
  state export --config /etc/zion/zion.yaml --out /backup/state-export.json
docker run --rm --user 10001:10001 --entrypoint sh \
  -v "$ZION_DATA_DIR:/source:ro" -v "$target:/backup" "$ZION_IMAGE" \
  -c 'umask 077; tar -czf /backup/node-data-sensitive.tar.gz -C /source .; sha256sum /backup/state-export.json /backup/node-data-sensitive.tar.gz > /backup/SHA256SUMS'
install -m 0644 "$ZION_CONFIG_DIR/bootstrap.yaml" "$target/bootstrap.yaml"
printf 'Sensitive node backup created at %s; store it encrypted and never publish it.\n' "$target"
if [[ "$keep_stopped" == true ]]; then
  was_running=false
fi
