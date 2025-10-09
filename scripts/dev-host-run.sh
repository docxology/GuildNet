#!/usr/bin/env sh
# Build and run the GuildNet host app locally with sensible defaults.
# - Builds the binary (make build)
# - Optionally generates shared dev TLS certs
# - Exports FRONTEND_ORIGIN if not set (for CORS)
# - Runs `./bin/hostapp serve`
#
# Usage:
#   scripts/dev-host-run.sh [--no-certs] [--origin URL]
#
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# Load shared env if present (.env at repo root)
if [ -f "$ROOT/.env" ]; then
  # shellcheck disable=SC1090
  . "$ROOT/.env"
fi
GEN_CERTS=1
ORIGIN=${FRONTEND_ORIGIN:-}

while [ $# -gt 0 ]; do
  case "$1" in
    --no-certs) GEN_CERTS=0; shift ;;
    --origin) ORIGIN="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

log() { printf "%s | %s\n" "$(date -Iseconds)" "$*"; }
need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }

need make

# Ensure build
log "Building hostapp..."
(
  cd "$ROOT"
  make build
)

# TLS certs
if [ $GEN_CERTS -eq 1 ]; then
  if [ -x "$ROOT/scripts/generate-server-cert.sh" ]; then
    "$ROOT/scripts/generate-server-cert.sh" -f
  else
    sh "$ROOT/scripts/generate-server-cert.sh" -f
  fi
fi

# FRONTEND_ORIGIN for CORS
if [ -z "$ORIGIN" ]; then
  ORIGIN="https://localhost:5173"
fi
export FRONTEND_ORIGIN="$ORIGIN"
# Pass DB connection env through if provided
if [ -n "${RETHINKDB_ADDR:-}" ]; then export RETHINKDB_ADDR; fi
if [ -n "${RETHINKDB_USER:-}" ]; then export RETHINKDB_USER; fi
if [ -n "${RETHINKDB_PASS:-}" ]; then export RETHINKDB_PASS; fi
log "FRONTEND_ORIGIN=$FRONTEND_ORIGIN"

# Ensure config exists (required for tsnet). If missing and env is provided, create non-interactively; otherwise run interactive init.
CFG="$HOME/.guildnet/config.json"
if [ ! -f "$CFG" ]; then
  if [ -n "${TS_LOGIN_SERVER:-}" ] && [ -n "${TS_AUTHKEY:-}" ] && [ -n "${TS_HOSTNAME:-}" ]; then
    log "Config not found; creating from environment (.env)"
    mkdir -p "$(dirname "$CFG")"
    cat >"$CFG" <<JSON
{
  "login_server": "${TS_LOGIN_SERVER}",
  "auth_key": "${TS_AUTHKEY}",
  "hostname": "${TS_HOSTNAME}",
  "listen_local": "${LISTEN_LOCAL}",
  "dial_timeout_ms": 3000,
  "allowlist": [],
  "name": "${CLUSTER_NAME:-}"
}
JSON
  else
    log "Config not found; launching interactive init (requires Login server URL, Pre-auth key, Hostname)"
    "$ROOT/bin/hostapp" init
  fi
# Tailscale (tsnet) is mandatory; ensure config/init provides LoginServer/AuthKey/Hostname.
fi

# Prefer 127.0.0.1:8080 unless already in use; allow override via LISTEN_LOCAL
LISTEN_LOCAL_DEFAULT="127.0.0.1:8080"
export LISTEN_LOCAL="${LISTEN_LOCAL:-$LISTEN_LOCAL_DEFAULT}"
log "LISTEN_LOCAL=$LISTEN_LOCAL"

# Default RETHINKDB_ADDR to local loopback for dev unless explicitly provided
if [ -z "${RETHINKDB_ADDR:-}" ]; then
  export RETHINKDB_ADDR="127.0.0.1:28015"
fi
log "RETHINKDB_ADDR=$RETHINKDB_ADDR"

# Ensure a local DB endpoint is available automatically
if ! (nc -z 127.0.0.1 28015 >/dev/null 2>&1); then
  if command -v kubectl >/dev/null 2>&1; then
    # Deploy RethinkDB Service/Deployment if missing (best-effort)
    if ! kubectl get svc rethinkdb >/dev/null 2>&1; then
      log "RethinkDB Service not found; applying $ROOT/k8s/rethinkdb.yaml"
      kubectl apply -f "$ROOT/k8s/rethinkdb.yaml" >/dev/null 2>&1 || true
      log "Waiting for deployment/rethinkdb rollout..."
      kubectl rollout status deployment/rethinkdb --timeout=60s >/dev/null 2>&1 || true
    fi
    # Start or reuse port-forward to local 28015
    if kubectl get svc rethinkdb >/dev/null 2>&1; then
      PF_FILE="$ROOT/.dev-rethinkdb-pf.pid"
      if [ -f "$PF_FILE" ]; then
        PF_PID=$(cat "$PF_FILE" 2>/dev/null || true)
        if [ -n "$PF_PID" ] && ps -p "$PF_PID" >/dev/null 2>&1; then
          log "Using existing port-forward (PID=$PF_PID)"
        else
          log "Starting kubectl port-forward: svc/rethinkdb -> 127.0.0.1:28015"
          kubectl port-forward svc/rethinkdb 28015:28015 >/dev/null 2>&1 & echo $! > "$PF_FILE" ; sleep 1
        fi
      else
        log "Starting kubectl port-forward: svc/rethinkdb -> 127.0.0.1:28015"
        kubectl port-forward svc/rethinkdb 28015:28015 >/dev/null 2>&1 & echo $! > "$PF_FILE" ; sleep 1
      fi
    else
      log "No Kubernetes RethinkDB service available; DB may remain unavailable."
    fi
  else
    log "kubectl not found; cannot auto-start DB port-forward."
  fi
fi

# Run server (tsnet mandatory)
exec "$ROOT/bin/hostapp" serve
