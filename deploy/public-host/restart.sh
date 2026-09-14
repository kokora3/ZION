#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/public-host/lib.sh
source "$SCRIPT_DIR/lib.sh"
load_settings
require_docker
before="$(status_json)"
before_peer="$(json_string_field "$before" peer_id)"
before_state="$(json_string_field "$before" state_hash)"
compose restart "$SERVICE_NAME"
wait_for_health
after="$(status_json)"
after_peer="$(json_string_field "$after" peer_id)"
after_state="$(json_string_field "$after" state_hash)"
[[ -n "$before_peer" && "$after_peer" == "$before_peer" ]] || die "PeerID changed across restart"
[[ -n "$before_state" && "$after_state" == "$before_state" ]] || die "StateHash changed across restart"
printf '%s\n' "$after"
printf 'Restart retained PeerID %s and StateHash %s.\n' "$after_peer" "$after_state"
