#!/usr/bin/env bash
# Runs cmd/server with the settings in .env.local (default) or the env file given as $1.
# The env file is sourced as shell code: quote values with spaces.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
env_file="${1:-$root/.env.local}"
[[ "$env_file" = /* ]] || env_file="$PWD/$env_file"  # $1 is relative to the caller's cwd
[[ -f "$env_file" ]] || { echo "missing $env_file" >&2; exit 1; }
set -a +u; . "$env_file"; set +a -u  # an unset $VAR in the env file stays empty, as in a shell
cd "$root"
# Open the web UI once the server answers; gives up after 60s (e.g. a build error).
url=http://localhost:8080/
( for _ in $(seq 60); do
    curl -fs -o /dev/null "$url" && { xdg-open "$url" >/dev/null 2>&1; exit; }
    sleep 1
  done ) &
exec go run ./cmd/server
