#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/public-host/lib.sh
source "$SCRIPT_DIR/lib.sh"
load_settings
require_docker

tail_lines="${ZION_LOG_TAIL:-200}"
[[ "$tail_lines" =~ ^[0-9]+$ ]] && (( tail_lines >= 1 && tail_lines <= 5000 )) || die "ZION_LOG_TAIL must be 1..5000"
if [[ "${1:-}" == --no-follow ]]; then
  compose logs --tail "$tail_lines" "$SERVICE_NAME"
else
  compose logs --tail "$tail_lines" --follow "$SERVICE_NAME"
fi
