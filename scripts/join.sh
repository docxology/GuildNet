#!/usr/bin/env bash
set -euo pipefail

# Join an existing GuildNet setup using a shared guildnet.config
# - Trusts optional CA for the Host App URL (curl only; system trust store unchanged)
# - Creates ~/.guildnet/config.json using provided tailscale pre-auth key and login server
# - Tests Host App /healthz and basic UI proxying readiness
#
# Usage:
#   scripts/join.sh /path/to/guildnet.config [--non-interactive]
#

if [ $# -lt 1 ]; then
  echo "Usage: $0 /path/to/guildnet.config [--non-interactive]" >&2
  exit 2
fi

CFG_FILE="${1:-}"
shift || true
NON_INTERACTIVE=0
if [ "${1:-}" = "--non-interactive" ]; then NON_INTERACTIVE=1; fi

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need jq
need curl

if [ ! -s "$CFG_FILE" ]; then echo "ERROR: config file not found: $CFG_FILE" >&2; exit 1; fi

JQ() { jq -r "$@" "$CFG_FILE" 2>/dev/null || true; }

HOSTAPP_URL="$(JQ '.hostapp.url // empty')"
if [ -z "$HOSTAPP_URL" ] || [ "$HOSTAPP_URL" = "null" ]; then
  HOSTAPP_URL="$(JQ '.ui.vite_api_base // empty')"
fi
HOSTAPP_CA_PEM="$(jq -r '.hostapp.ca_pem // empty' "$CFG_FILE" 2>/dev/null || true)"
if [ -z "$HOSTAPP_CA_PEM" ] || [ "$HOSTAPP_CA_PEM" = "null" ]; then
  HOSTAPP_CA_PEM="$(jq -r '.ui.ca_pem // empty' "$CFG_FILE" 2>/dev/null || true)"
fi
TS_LOGIN="$(JQ '.tailscale.login_server // empty')"
TS_KEY="$(JQ '.tailscale.preauth_key // empty')"
TS_HOSTNAME="$(JQ '.tailscale.hostname // empty')"
NAME_LABEL="$(JQ '.name // empty')"

if [ -z "$HOSTAPP_URL" ]; then echo "ERROR: hostapp.url missing in config" >&2; exit 1; fi

echo "Join target: $HOSTAPP_URL" >&2
if [ -n "$NAME_LABEL" ]; then echo "Name: $NAME_LABEL" >&2; fi
if [ -n "$TS_LOGIN" ]; then echo "Login server: $TS_LOGIN" >&2; fi
if [ -n "$TS_HOSTNAME" ]; then echo "Hostname: $TS_HOSTNAME" >&2; fi

# Prepare curl CA option
TMP_CA=""
CURL_CA_ARGS=()
if [ -n "$HOSTAPP_CA_PEM" ] && [ "$HOSTAPP_CA_PEM" != "null" ]; then
  TMP_CA="$(mktemp)"
  printf "%s" "$HOSTAPP_CA_PEM" > "$TMP_CA"
  CURL_CA_ARGS=(--cacert "$TMP_CA")
else
  # For local/self-signed dev, allow -k silently
  CURL_CA_ARGS=(-k)
fi

# Verify /healthz
echo "Checking Host App health..." >&2
if ! curl -sS "${CURL_CA_ARGS[@]}" "$HOSTAPP_URL/healthz" | grep -q "ok"; then
  echo "ERROR: Host App /healthz failed at $HOSTAPP_URL" >&2
  exit 1
fi
echo "OK: Host App is healthy" >&2

# Write ~/.guildnet/config.json for the joiner if pre-auth is provided
GN_DIR="$HOME/.guildnet"
mkdir -p "$GN_DIR/state/certs" || true
OUT_CFG="$GN_DIR/config.json"

LISTEN_LOCAL_DEFAULT="127.0.0.1:8080"
if [ -n "$TS_LOGIN" ] && [ -n "$TS_KEY" ]; then
  # Choose a default hostname when not suggested
  if [ -z "$TS_HOSTNAME" ] || [ "$TS_HOSTNAME" = "null" ]; then
    TS_HOSTNAME="guildnet-$(whoami 2>/dev/null || echo user)-$(date +%Y%m%d%H%M%S)"
  fi
  cat >"$OUT_CFG" <<JSON
{
  "login_server": "$TS_LOGIN",
  "auth_key": "$TS_KEY",
  "hostname": "$TS_HOSTNAME",
  "listen_local": "$LISTEN_LOCAL_DEFAULT",
  "dial_timeout_ms": 3000,
  "name": ${NAME_LABEL:+"$NAME_LABEL"}
}
JSON
  chmod 600 "$OUT_CFG" || true
  echo "Wrote tsnet config: $OUT_CFG" >&2
else
  echo "No pre-auth key provided in config; skipping tsnet config write." >&2
fi

# Optionally place CA to ~/.guildnet/state/certs for the local server use
if [ -n "$TMP_CA" ] && [ -s "$TMP_CA" ]; then
  cp "$TMP_CA" "$GN_DIR/state/certs/hostapp-ca.pem"
fi

echo
echo "Join complete." >&2
echo "Next steps:" >&2
echo "  - Start the Host App locally if you plan to run your own instance: make dev-backend" >&2
echo "  - Open the shared Host App in your browser: $HOSTAPP_URL" >&2
echo "  - Optional: run 'make dev-ui' for a local UI (it will talk to VITE_API_BASE=$HOSTAPP_URL)" >&2

rm -f "$TMP_CA" 2>/dev/null || true
