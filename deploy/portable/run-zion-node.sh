#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"
mkdir -p "$script_dir/data/normal"
exec "$script_dir/zion-node" run \
  --config "$script_dir/configs/normal.yaml" \
  --data-dir "$script_dir/data/normal"

