#!/usr/bin/env bash
set -euo pipefail

PUBLIC_HOST_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPOSITORY_ROOT="$(cd -- "$PUBLIC_HOST_DIR/../.." && pwd -P)"
ENV_FILE="${ZION_ENV_FILE:-$PUBLIC_HOST_DIR/.env}"
SERVICE_NAME="zion-node-bootstrap"
CONTAINER_NAME="zion-node-bootstrap"

die() {
  printf 'D2A: %s\n' "$*" >&2
  exit 1
}

read_env_value() {
  local key="$1"
  awk -v wanted="$key" '
    /^[[:space:]]*(#|$)/ { next }
    index($0, wanted "=") == 1 { sub(/^[^=]*=/, ""); sub(/\r$/, ""); value=$0 }
    END { if (value != "") print value }
  ' "$ENV_FILE"
}

load_setting() {
  local key="$1" default_value="${2-}" current_value="${!key-}"
  if [[ -z "$current_value" ]]; then
    current_value="$(read_env_value "$key")"
  fi
  if [[ -z "$current_value" ]]; then
    current_value="$default_value"
  fi
  printf -v "$key" '%s' "$current_value"
  export "$key"
}

load_settings() {
  [[ -f "$ENV_FILE" ]] || die "missing $ENV_FILE; copy .env.example to .env"
  load_setting ZION_IMAGE
  load_setting ZION_DEPLOY_MODE release
  load_setting ZION_DATA_DIR /opt/zion/data
  load_setting ZION_CONFIG_DIR /opt/zion/config
  load_setting ZION_BACKUP_DIR /opt/zion/backups
  load_setting ZION_PUBLIC_HOST
  load_setting ZION_PUBLIC_HOST_KIND dns4
  load_setting ZION_P2P_PORT 42000
  load_setting ZION_COMPOSE_PROJECT zion-public

  [[ -n "$ZION_IMAGE" && "$ZION_IMAGE" != *[[:space:]]* ]] || die "ZION_IMAGE must be a non-empty image reference"
  [[ "$ZION_IMAGE" != *:latest ]] || die "ZION_IMAGE=latest is forbidden; pin a version or digest"
  local image_tail="${ZION_IMAGE##*/}"
  [[ "$image_tail" == *:* || "$ZION_IMAGE" == *@sha256:* ]] || die "ZION_IMAGE must include an explicit tag or digest"
  [[ "$ZION_DEPLOY_MODE" == release || "$ZION_DEPLOY_MODE" == local ]] || die "ZION_DEPLOY_MODE must be release or local"
  [[ "$ZION_COMPOSE_PROJECT" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || die "invalid ZION_COMPOSE_PROJECT"
  [[ "$ZION_P2P_PORT" =~ ^[0-9]+$ ]] && (( ZION_P2P_PORT >= 1 && ZION_P2P_PORT <= 65535 )) || die "ZION_P2P_PORT must be 1..65535"
  case "$ZION_PUBLIC_HOST_KIND" in
    dns4|dns6) [[ "$ZION_PUBLIC_HOST" =~ ^[A-Za-z0-9.-]+$ ]] || die "invalid DNS host" ;;
    ip4) [[ "$ZION_PUBLIC_HOST" =~ ^[0-9.]+$ ]] || die "invalid IPv4 text" ;;
    ip6) [[ "$ZION_PUBLIC_HOST" =~ ^[0-9A-Fa-f:]+$ ]] || die "invalid IPv6 text" ;;
    *) die "ZION_PUBLIC_HOST_KIND must be dns4, dns6, ip4, or ip6" ;;
  esac
  for path_value in "$ZION_DATA_DIR" "$ZION_CONFIG_DIR" "$ZION_BACKUP_DIR"; do
    [[ "$path_value" == /* && "$path_value" != / && "$path_value" != *$'\n'* ]] || die "deployment paths must be absolute non-root Linux paths"
  done
  [[ "$ZION_DATA_DIR" != "$ZION_CONFIG_DIR" && "$ZION_DATA_DIR" != "$ZION_BACKUP_DIR" && "$ZION_CONFIG_DIR" != "$ZION_BACKUP_DIR" ]] || die "data, config, and backup paths must be distinct"

  ZION_PUBLIC_MULTIADDR="/$ZION_PUBLIC_HOST_KIND/$ZION_PUBLIC_HOST/udp/$ZION_P2P_PORT/quic-v1"
  export ZION_PUBLIC_MULTIADDR
}

compose() {
  docker compose --env-file "$ENV_FILE" \
    --project-directory "$REPOSITORY_ROOT" \
    --project-name "$ZION_COMPOSE_PROJECT" \
    -f "$REPOSITORY_ROOT/docker-compose.yml" \
    -f "$PUBLIC_HOST_DIR/compose.override.yml" \
    --profile bootstrap "$@"
}

require_docker() {
  [[ "$(uname -s)" == Linux ]] || die "public-host helpers require Linux"
  command -v docker >/dev/null 2>&1 || die "Docker Engine is unavailable"
  docker compose version >/dev/null 2>&1 || die "Docker Compose plugin is unavailable"
  docker info >/dev/null 2>&1 || die "Docker daemon is unavailable to this operator"
}

verify_shared_identity() {
  local genesis_file="$REPOSITORY_ROOT/configs/alpha-1/genesis-id.txt"
  local config_source="$PUBLIC_HOST_DIR/bootstrap.yaml"
  [[ -f "$genesis_file" && -f "$REPOSITORY_ROOT/configs/alpha-1/genesis.json" && -f "$REPOSITORY_ROOT/configs/alpha-1/validators-public.json" ]] || die "G1 shared genesis files are missing"
  SHARED_GENESIS_ID="$(tr -d '\r\n' < "$genesis_file")"
  [[ "$SHARED_GENESIS_ID" =~ ^[0-9a-f]{64}$ ]] || die "G1 genesis-id.txt is invalid"
  grep -Fxq 'network_id: zion-alpha-1' "$config_source" || die "public-host config has the wrong NetworkID"
  grep -Fxq "genesis_id: $SHARED_GENESIS_ID" "$config_source" || die "public-host config does not consume the frozen G1 GenesisID"
  grep -Fxq 'roles: [NORMAL, BOOTSTRAP]' "$config_source" || die "public-host config must be NORMAL + BOOTSTRAP"
  grep -Fxq '  enabled: false' "$config_source" || die "public-host config unexpectedly enables consensus"
  export SHARED_GENESIS_ID
}

prepare_host_paths() {
  mkdir -p -- "$ZION_DATA_DIR" "$ZION_CONFIG_DIR" "$ZION_BACKUP_DIR"
  chmod 0700 "$ZION_DATA_DIR" "$ZION_BACKUP_DIR"
  chmod 0755 "$ZION_CONFIG_DIR"
  local installed="$ZION_CONFIG_DIR/bootstrap.yaml"
  if [[ -e "$installed" ]]; then
    cmp -s "$PUBLIC_HOST_DIR/bootstrap.yaml" "$installed" || die "$installed differs from the reviewed D2A config; resolve it explicitly"
  else
    install -m 0644 "$PUBLIC_HOST_DIR/bootstrap.yaml" "$installed"
  fi
}

ensure_image() {
  if [[ "$ZION_DEPLOY_MODE" == release ]]; then
    docker pull "$ZION_IMAGE"
  else
    compose build "$SERVICE_NAME"
  fi
}

ensure_runtime_ownership() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    chown 10001:10001 "$ZION_DATA_DIR" "$ZION_BACKUP_DIR"
  else
    docker run --rm --user 0 --entrypoint chown \
      -v "$ZION_DATA_DIR:/var/lib/zion" -v "$ZION_BACKUP_DIR:/var/backups/zion" \
      "$ZION_IMAGE" 10001:10001 /var/lib/zion /var/backups/zion
  fi
  docker run --rm --user 10001:10001 --entrypoint sh -v "$ZION_DATA_DIR:/var/lib/zion" "$ZION_IMAGE" -c 'test -w /var/lib/zion' \
    || die "ZION_DATA_DIR is not writable by container UID 10001"
}

verify_with_image() {
  docker run --rm --entrypoint zionctl \
    -v "$REPOSITORY_ROOT/configs/alpha-1:/genesis:ro" "$ZION_IMAGE" \
    genesis verify --manifest /genesis/validators-public.json --genesis /genesis/genesis.json --genesis-id /genesis/genesis-id.txt --json
  compose config --quiet
  compose run --rm --no-deps "$SERVICE_NAME" \
    zion-node config validate --config /etc/zion/zion.yaml --p2p-advertise "$ZION_PUBLIC_MULTIADDR" \
      --expected-network zion-alpha-1 --expected-genesis-id "$SHARED_GENESIS_ID"
}

wait_for_health() {
  local attempt state
  for attempt in $(seq 1 24); do
    state="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$CONTAINER_NAME" 2>/dev/null || true)"
    case "$state" in
      healthy) return 0 ;;
      unhealthy) compose logs --tail 200 "$SERVICE_NAME" >&2; die "container is unhealthy" ;;
    esac
    sleep 5
  done
  compose logs --tail 200 "$SERVICE_NAME" >&2 || true
  die "container did not become healthy"
}

status_json() {
  compose exec -T "$SERVICE_NAME" zionctl --bearer-token-file /var/lib/zion/runtime/api-token status
}

json_string_field() {
  local json="$1" field="$2"
  printf '%s\n' "$json" | sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p"
}
