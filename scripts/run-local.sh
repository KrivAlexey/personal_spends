#!/usr/bin/env bash
# Runs cmd/server with the settings in .env.local (default) or the env file given as $1.
set -euo pipefail
cd "$(dirname "$0")/.."
env_file="${1:-.env.local}"
[[ -f "$env_file" ]] || { echo "missing $env_file" >&2; exit 1; }
set -a; . "./$env_file"; set +a
exec go run ./cmd/server
