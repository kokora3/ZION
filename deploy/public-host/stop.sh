#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/public-host/lib.sh
source "$SCRIPT_DIR/lib.sh"
load_settings
require_docker
compose stop "$SERVICE_NAME"
printf 'ZION stopped; data, identity, state, objects, and backups were retained.\n'
