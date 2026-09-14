#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/public-host/lib.sh
source "$SCRIPT_DIR/lib.sh"
load_settings
require_docker
verify_shared_identity

compose ps "$SERVICE_NAME"
status="$(status_json)"
printf '%s\n' "$status"
peer_id="$(json_string_field "$status" peer_id)"
[[ -n "$peer_id" ]] || die "runtime status omitted PeerID"
printf 'Bootstrap Multiaddr: %s/p2p/%s\n' "$ZION_PUBLIC_MULTIADDR" "$peer_id"
