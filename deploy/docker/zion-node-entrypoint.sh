#!/bin/sh
set -eu

umask 077
token_file="${ZION_API_TOKEN_FILE:-/var/lib/zion/runtime/api-token}"
token_dir="${token_file%/*}"
mkdir -p "$token_dir"
if [ ! -s "$token_file" ]; then
  # The bearer credential is created at container start in persistent storage;
  # it is never part of an image layer or Compose environment value.
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n' > "$token_file"
fi
chmod 0600 "$token_file"

exec "$@"

