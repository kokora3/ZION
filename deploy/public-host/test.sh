#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPOSITORY_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd -P)"
RUNTIME_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/zion-d2a.XXXXXXXX")"
TEST_ENV="$RUNTIME_ROOT/public-host.env"
TEST_IMAGE="${ZION_TEST_IMAGE:-zion-node:v0.1.0-alpha.1}"
printf '%s\n' \
  "ZION_IMAGE=$TEST_IMAGE" \
  'ZION_DEPLOY_MODE=local' \
  "ZION_DATA_DIR=$RUNTIME_ROOT/bootstrap-data" \
  "ZION_CONFIG_DIR=$RUNTIME_ROOT/config" \
  "ZION_BACKUP_DIR=$RUNTIME_ROOT/backups" \
  'ZION_PUBLIC_HOST=zion-node-bootstrap' \
  'ZION_PUBLIC_HOST_KIND=dns4' \
  'ZION_P2P_PORT=42000' \
  'ZION_COMPOSE_PROJECT=zion-d2a-ci' >"$TEST_ENV"
export ZION_ENV_FILE="$TEST_ENV"
# shellcheck source=deploy/public-host/lib.sh
source "$SCRIPT_DIR/lib.sh"
load_settings
CONTAINER_NAME=zion-d2a-ci-bootstrap
ZION_TEST_NORMAL_DATA_DIR="$RUNTIME_ROOT/normal-data"
ZION_TEST_NORMAL_CONFIG="$RUNTIME_ROOT/config/normal.yaml"
export ZION_TEST_NORMAL_DATA_DIR ZION_TEST_NORMAL_CONFIG

test_compose() {
  compose --profile normal -f "$SCRIPT_DIR/compose.test.yml" "$@"
}

cleanup() {
  docker rm -f zion-d2a-port-blocker >/dev/null 2>&1 || true
  docker rm -f zion-d2a-unhealthy >/dev/null 2>&1 || true
  test_compose down --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

require_docker
verify_shared_identity
mkdir -p "$ZION_TEST_NORMAL_DATA_DIR"
prepare_host_paths
docker run --rm --user 0 --entrypoint chown \
  -v "$ZION_DATA_DIR:/bootstrap" -v "$ZION_TEST_NORMAL_DATA_DIR:/normal" \
  "$ZION_IMAGE" 10001:10001 /bootstrap /normal

cp "$SCRIPT_DIR/bootstrap.yaml" "$ZION_TEST_NORMAL_CONFIG"
sed -i 's/roles: \[NORMAL, BOOTSTRAP\]/roles: [NORMAL]/' "$ZION_TEST_NORMAL_CONFIG"
test_compose config --quiet
rendered="$(test_compose config)"
[[ "$rendered" == *'protocol: udp'* && "$rendered" != *'published: "42001"'* && "$rendered" != *'published: 42001'* ]] \
  || die "Compose did not preserve the UDP-only public boundary"

test_compose up -d --no-build "$SERVICE_NAME"
wait_for_health
bootstrap_before="$(status_json)"
bootstrap_peer="$(json_string_field "$bootstrap_before" peer_id)"
bootstrap_state="$(json_string_field "$bootstrap_before" state_hash)"
[[ -n "$bootstrap_peer" && "$bootstrap_before" == *'"validator_authorized":false'* ]] || die "bootstrap gained authority or omitted PeerID"
[[ -z "$(docker port "$CONTAINER_NAME" 42001/tcp 2>/dev/null || true)" ]] || die "API port was published"
[[ -n "$(docker port "$CONTAINER_NAME" 42000/udp)" ]] || die "QUIC/UDP port was not published"

bootstrap_multiaddr="$ZION_PUBLIC_MULTIADDR/p2p/$bootstrap_peer"
sed -i "s|  bootstrap_addresses: \[\]|  bootstrap_addresses: [$bootstrap_multiaddr]|" "$ZION_TEST_NORMAL_CONFIG"
test_compose up -d --no-build zion-node-normal
for attempt in $(seq 1 24); do
  normal_status="$(test_compose exec -T zion-node-normal zionctl --bearer-token-file /var/lib/zion/runtime/api-token status 2>/dev/null || true)"
  normal_peers="$(test_compose exec -T zion-node-normal zionctl --bearer-token-file /var/lib/zion/runtime/api-token peers 2>/dev/null || true)"
  if [[ "$normal_status" == *'"runtime_state":"RUNNING"'* && "$normal_peers" == *"$bootstrap_peer"* ]]; then
    break
  fi
  sleep 5
done
[[ "$normal_peers" == *"$bootstrap_peer"* ]] || die "NORMAL did not connect to BOOTSTRAP"

test_compose rm -sf "$SERVICE_NAME"
test_compose up -d --no-build "$SERVICE_NAME"
wait_for_health
bootstrap_after="$(status_json)"
[[ "$(json_string_field "$bootstrap_after" peer_id)" == "$bootstrap_peer" ]] || die "PeerID changed after container recreation"
[[ "$(json_string_field "$bootstrap_after" state_hash)" == "$bootstrap_state" ]] || die "StateHash changed after container recreation"

bad_genesis="$RUNTIME_ROOT/config/bad-genesis.yaml"
sed -e 's/^genesis_id: .*/genesis_id: 4cce10c9eb93aa5baff6ec94b13ff27662464668764a39efed6d59373a487d55/' \
  -e 's|^  bearer_token_file:.*|  bearer_token_file: ""|' "$SCRIPT_DIR/bootstrap.yaml" >"$bad_genesis"
if docker run --rm --entrypoint zion-node -v "$bad_genesis:/etc/zion/zion.yaml:ro" "$ZION_IMAGE" \
  config validate --config /etc/zion/zion.yaml --expected-network zion-alpha-1 --expected-genesis-id "$SHARED_GENESIS_ID"; then
  die "local-only GenesisID passed the public-host validator"
fi
wrong_network="$RUNTIME_ROOT/config/wrong-network.yaml"
sed -e 's/^network_id: .*/network_id: another-network/' \
  -e 's|^  bearer_token_file:.*|  bearer_token_file: ""|' "$SCRIPT_DIR/bootstrap.yaml" >"$wrong_network"
if docker run --rm --entrypoint zion-node -v "$wrong_network:/etc/zion/zion.yaml:ro" "$ZION_IMAGE" \
  config validate --config /etc/zion/zion.yaml --expected-network zion-alpha-1; then
  die "wrong NetworkID passed the public-host validator"
fi
invalid_config="$RUNTIME_ROOT/config/invalid.yaml"
sed 's|^  bearer_token_file:.*|  bearer_token_file: ""|' "$SCRIPT_DIR/bootstrap.yaml" >"$invalid_config"
printf 'unknown_d2a_field: true\n' >>"$invalid_config"
if docker run --rm --entrypoint zion-node -v "$invalid_config:/etc/zion/zion.yaml:ro" "$ZION_IMAGE" config validate --config /etc/zion/zion.yaml; then
  die "invalid config passed validation"
fi

missing_root="$RUNTIME_ROOT/missing-g1"
mkdir -p "$missing_root/deploy/public-host"
cp "$SCRIPT_DIR/lib.sh" "$missing_root/deploy/public-host/lib.sh"
cp "$SCRIPT_DIR/bootstrap.yaml" "$missing_root/deploy/public-host/bootstrap.yaml"
if (
  export ZION_ENV_FILE="$TEST_ENV"
  # shellcheck source=/dev/null
  source "$missing_root/deploy/public-host/lib.sh"
  load_settings
  verify_shared_identity
); then
  die "missing G1 files passed shared-identity validation"
fi

readonly_data="$RUNTIME_ROOT/read-only-data"
mkdir -p "$readonly_data"
chmod 0555 "$readonly_data"
if docker run --rm --user 10001:10001 --entrypoint sh -v "$readonly_data:/data" "$ZION_IMAGE" -c 'test -w /data'; then
  die "unwritable data directory passed the runtime ownership boundary"
fi
chmod 0755 "$readonly_data"

no_tools_bin="$RUNTIME_ROOT/no-tools-bin"
mkdir -p "$no_tools_bin"
printf '#!/bin/sh\nprintf "Linux\\n"\n' >"$no_tools_bin/uname"
chmod 0755 "$no_tools_bin/uname"
if ( PATH="$no_tools_bin"; require_docker ); then
  die "missing Docker passed dependency validation"
fi
printf '#!/bin/sh\nexit 1\n' >"$no_tools_bin/docker"
chmod 0755 "$no_tools_bin/docker"
if ( PATH="$no_tools_bin"; require_docker ); then
  die "unavailable Docker Compose passed dependency validation"
fi

docker run -d --name zion-d2a-unhealthy --health-cmd false --health-interval 1s --health-timeout 1s --health-retries 1 \
  --entrypoint sleep "$ZION_IMAGE" 30 >/dev/null
if ( CONTAINER_NAME=zion-d2a-unhealthy; wait_for_health ); then
  die "unhealthy container passed health validation"
fi
docker rm -f zion-d2a-unhealthy >/dev/null

test_compose stop "$SERVICE_NAME"
docker run -d --name zion-d2a-port-blocker -p "$ZION_P2P_PORT:42000/udp" --entrypoint sleep "$ZION_IMAGE" 60 >/dev/null
if test_compose up -d --no-build "$SERVICE_NAME"; then
  die "occupied public UDP port did not fail visibly"
fi
docker rm -f zion-d2a-port-blocker >/dev/null
test_compose up -d --no-build "$SERVICE_NAME"
wait_for_health
[[ "$(json_string_field "$(status_json)" peer_id)" == "$bootstrap_peer" ]] || die "port-conflict recovery changed PeerID"

if grep -Eni '(digitalocean|droplet|aws|ec2|azure|hetzner|linode)' \
  "$SCRIPT_DIR/lib.sh" "$SCRIPT_DIR/deploy.sh" "$SCRIPT_DIR/status.sh" "$SCRIPT_DIR/logs.sh" \
  "$SCRIPT_DIR/stop.sh" "$SCRIPT_DIR/restart.sh" "$SCRIPT_DIR/update.sh" "$SCRIPT_DIR/backup.sh" \
  "$SCRIPT_DIR/compose.override.yml" "$SCRIPT_DIR/bootstrap.yaml" "$SCRIPT_DIR/.env.example"; then
  die "provider dependency found in runtime deployment material"
fi
if grep -Eni '(BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})' \
  "$SCRIPT_DIR/lib.sh" "$SCRIPT_DIR/deploy.sh" "$SCRIPT_DIR/status.sh" "$SCRIPT_DIR/logs.sh" \
  "$SCRIPT_DIR/stop.sh" "$SCRIPT_DIR/restart.sh" "$SCRIPT_DIR/update.sh" "$SCRIPT_DIR/backup.sh" \
  "$SCRIPT_DIR/compose.override.yml" "$SCRIPT_DIR/bootstrap.yaml" "$SCRIPT_DIR/.env.example"; then
  die "secret-like material found in public-host deployment"
fi
printf 'D2A Docker simulation passed: NORMAL connected, PeerID/StateHash persisted, failures were safe.\n'
