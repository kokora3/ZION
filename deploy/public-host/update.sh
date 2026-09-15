#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/public-host/lib.sh
source "$SCRIPT_DIR/lib.sh"
load_settings
require_docker
verify_shared_identity

before="$(status_json)"
before_peer="$(json_string_field "$before" peer_id)"
before_state="$(json_string_field "$before" state_hash)"
before_network="$(json_string_field "$before" network_id)"
before_genesis="$(json_string_field "$before" genesis_id)"
current_image="$(docker inspect --format '{{.Config.Image}}' "$CONTAINER_NAME")"
[[ -n "$current_image" ]] || die "cannot determine the running image for pre-update backup"
ZION_IMAGE="$current_image" bash "$SCRIPT_DIR/backup.sh" --keep-stopped
ensure_image
verify_with_image
compose up -d --no-build --force-recreate "$SERVICE_NAME"
wait_for_health
after="$(status_json)"
[[ "$(json_string_field "$after" peer_id)" == "$before_peer" ]] || die "PeerID changed during update"
[[ "$(json_string_field "$after" state_hash)" == "$before_state" ]] || die "StateHash changed during update"
[[ "$(json_string_field "$after" network_id)" == "$before_network" ]] || die "NetworkID changed during update"
[[ "$(json_string_field "$after" genesis_id)" == "$before_genesis" && "$before_genesis" == "$SHARED_GENESIS_ID" ]] || die "GenesisID changed during update"
printf '%s\n' "$after"
printf 'Update retained NetworkID, GenesisID, PeerID, and StateHash.\n'
