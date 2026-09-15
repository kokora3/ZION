#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/public-host/lib.sh
source "$SCRIPT_DIR/lib.sh"

mode="${1:-deploy}"
[[ "$mode" == deploy || "$mode" == --check ]] || die "usage: ./deploy.sh [--check]"
load_settings
require_docker
verify_shared_identity
prepare_host_paths
ensure_image
ensure_runtime_ownership
verify_with_image

if [[ "$mode" == --check ]]; then
  printf 'D2A public-host configuration is valid for %s (%s).\n' "$ZION_PUBLIC_MULTIADDR" "$SHARED_GENESIS_ID"
  exit 0
fi

compose up -d --no-build "$SERVICE_NAME"
wait_for_health
bash "$SCRIPT_DIR/status.sh"
