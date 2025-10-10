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
# Full-cluster deploy defaults (edit as needed)
CLUSTER=${CLUSTER:-mycluster}
ENDPOINT=${ENDPOINT:-https://10.0.0.10:6443}
CP_NODES=${CP_NODES:-10.0.0.10}
WK_NODES=${WK_NODES:-10.0.0.20}
# Deploy tuning
PRECHECK_PORT=${PRECHECK_PORT:-50000}
PRECHECK_TIMEOUT=${PRECHECK_TIMEOUT:-3}
PRECHECK_MAX_WAIT_SECS=${PRECHECK_MAX_WAIT_SECS:-600}
PRECHECK_PING=${PRECHECK_PING:-0}
REQUIRE_ENDPOINT_MATCH_CP=${REQUIRE_ENDPOINT_MATCH_CP:-0}
APPLY_RETRIES=${APPLY_RETRIES:-10}
APPLY_RETRY_DELAY=${APPLY_RETRY_DELAY:-5}
KUBE_READY_TRIES=${KUBE_READY_TRIES:-90}
KUBE_READY_DELAY=${KUBE_READY_DELAY:-5}
# DB setup toggle
DB_SETUP=${DB_SETUP:-1}
RETHINKDB_SERVICE_NAME=${RETHINKDB_SERVICE_NAME:-rethinkdb}
RETHINKDB_NAMESPACE=${RETHINKDB_NAMESPACE:-}
ENV

log "Wrote $OUT"
