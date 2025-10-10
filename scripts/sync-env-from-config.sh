#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CFG="$HOME/.guildnet/config.json"
OUT="$ROOT/.env"

log(){ printf "%s | %s\n" "$(date -Iseconds)" "$*"; }

if [ ! -f "$CFG" ]; then
  echo "No config at $CFG; run hostapp init." >&2
  exit 1
fi

login=$(jq -r '.login_server // empty' "$CFG")
auth=$(jq -r '.auth_key // empty' "$CFG")
host=$(jq -r '.hostname // empty' "$CFG")

if [ -z "$login" ] || [ -z "$auth" ] || [ -z "$host" ]; then
  echo "Missing fields in $CFG" >&2
  exit 1
fi

cat > "$OUT" <<ENV
# Shared Tailscale/Headscale config for GuildNet
TS_LOGIN_SERVER=${login}
TS_AUTHKEY=${auth}
TS_HOSTNAME=${host}
# Default routes for Talos cluster and services
TS_ROUTES=${TS_ROUTES:-10.96.0.0/12,10.244.0.0/16}
# Optional Headscale aliases
HEADSCALE_URL=${HEADSCALE_URL:-}
HEADSCALE_AUTHKEY=${HEADSCALE_AUTHKEY:-}
# Optional cluster name
CLUSTER_NAME=${CLUSTER_NAME:-guildnet}
ENV

log "Wrote $OUT"
