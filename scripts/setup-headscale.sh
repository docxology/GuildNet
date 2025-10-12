#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need docker

# Start Headscale (Docker) and sync .env with reachable URL
bash "$ROOT/scripts/headscale-run.sh" up
bash "$ROOT/scripts/detect-lan-and-sync-env.sh" || true

# Bootstrap: create user+preauth key and write TS_AUTHKEY into .env
bash "$ROOT/scripts/headscale-bootstrap.sh"

# Show .env highlights (sanitized)
echo "\n.env headscale settings (sanitized):"
(egrep -n '^(TS_LOGIN_SERVER|TS_AUTHKEY|HEADSCALE_URL)=' .env || true) | sed 's/=.*/=***/'

echo "Headscale setup complete."