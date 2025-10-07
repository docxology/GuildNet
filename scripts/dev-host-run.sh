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

# Run server (tsnet mandatory)
exec "$ROOT/bin/hostapp" serve
