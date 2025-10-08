#!/usr/bin/env bash
set -euo pipefail

# Create a portable GuildNet join configuration (guildnet.config)
# This captures:
# - Host App URL reachable over the tailnet (https://<ts-fqdn>:443)
# - Optional CA PEM to trust that Host App URL
# - Headscale/Tailscale login server and a pre-auth key (sensitive)
# - Suggested hostname and display name
#
# Usage examples:
#   scripts/create_join_info.sh \
#     --hostapp-url https://myhost.tailnet-abc.ts.net:443 \
#     --include-ca certs/server.crt \
#     --login-server https://headscale.example.com \
#     --auth-key tskey-abc123 \
#     --hostname teammate-1 \
#     --name "Dev Cluster" \
#     --out guildnet.config
#
# Notes:
# - The output contains a pre-auth key if provided. Handle securely and share out-of-band.
# - File is written with permissions 0600.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Defaults
OUT_FILE="guildnet.config"
HOSTAPP_URL=""
INCLUDE_CA=""
LOGIN_SERVER=""
AUTH_KEY=""
HOSTNAME_SUGGEST=""
NAME_LABEL=""
NOTES=""

usage() {
  cat <<USAGE
Usage: scripts/create_join_info.sh [options]

Options:
  --hostapp-url URL       Host App URL reachable over tailnet (e.g., https://<ts-fqdn>:443)
  --include-ca PATH       Include PEM-encoded CA/cert at PATH (e.g., certs/server.crt)
  --login-server URL      Headscale/Tailscale login server URL
  --auth-key KEY          Pre-auth key (sensitive)
  --hostname NAME         Suggested tsnet hostname for the joiner
  --name LABEL            Optional display name/cluster label shown in UI
  --notes TEXT            Optional free-form note/instructions
  --out FILE              Output path (default: guildnet.config)
  --help                  Show this help
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --hostapp-url) HOSTAPP_URL="${2:-}"; shift 2 ;;
    --include-ca) INCLUDE_CA="${2:-}"; shift 2 ;;
    --login-server) LOGIN_SERVER="${2:-}"; shift 2 ;;
    --auth-key) AUTH_KEY="${2:-}"; shift 2 ;;
    --hostname) HOSTNAME_SUGGEST="${2:-}"; shift 2 ;;
    --name) NAME_LABEL="${2:-}"; shift 2 ;;
    --notes) NOTES="${2:-}"; shift 2 ;;
    --out) OUT_FILE="${2:-}"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown arg: $1" >&2; usage; exit 2 ;;
  esac
done

# Try to pull defaults from ~/.guildnet/config.json when not provided
CONF_FILE="$HOME/.guildnet/config.json"
if [ -z "$LOGIN_SERVER" ] && [ -s "$CONF_FILE" ] && command -v jq >/dev/null 2>&1; then
  LOGIN_SERVER="$(jq -r '.login_server // empty' "$CONF_FILE" 2>/dev/null || true)"
fi
if [ -z "$HOSTNAME_SUGGEST" ] && [ -s "$CONF_FILE" ] && command -v jq >/dev/null 2>&1; then
  HOSTNAME_SUGGEST="$(jq -r '.hostname // empty' "$CONF_FILE" 2>/dev/null || true)"
fi
if [ -z "$NAME_LABEL" ] && [ -s "$CONF_FILE" ] && command -v jq >/dev/null 2>&1; then
  NAME_LABEL="$(jq -r '.name // empty' "$CONF_FILE" 2>/dev/null || true)"
fi

# Read CA PEM if requested or fallback to repo certs/server.crt
CA_PEM=""
if [ -n "$INCLUDE_CA" ]; then
  if [ -f "$INCLUDE_CA" ]; then
    CA_PEM="$(cat "$INCLUDE_CA")"
  else
    echo "WARN: --include-ca path not found: $INCLUDE_CA" >&2
  fi
elif [ -f "$REPO_ROOT/certs/server.crt" ]; then
  CA_PEM="$(cat "$REPO_ROOT/certs/server.crt")"
fi

# Minimal validation
if [ -z "$HOSTAPP_URL" ]; then
  echo "ERROR: --hostapp-url is required" >&2; exit 2
fi

# Build JSON
NOW_ISO="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
HOSTNAME_LOCAL="$(hostname 2>/dev/null || echo unknown)"
CREATOR_USER="${USER:-unknown}"

TMP_JSON="$(mktemp)"
cat >"$TMP_JSON" <<JSON
{
  "version": 1,
  "created_at": "$NOW_ISO",
  "creator": {"host": "$HOSTNAME_LOCAL", "user": "$CREATOR_USER"},
  "hostapp": {"url": "$HOSTAPP_URL"$( [ -n "$CA_PEM" ] && printf ", \"ca_pem\": %s" "$(jq -Rs . <<<"$CA_PEM")" )},
  "tailscale": {
    "login_server": $(jq -Rn --arg v "$LOGIN_SERVER" '$v // empty'),
    "preauth_key": $(jq -Rn --arg v "$AUTH_KEY" '$v // empty'),
    "hostname_suggest": $(jq -Rn --arg v "$HOSTNAME_SUGGEST" '$v // empty')
  },
  "ui": {"vite_api_base": "$HOSTAPP_URL"},
  "name": $(jq -Rn --arg v "$NAME_LABEL" '$v // empty'),
  "notes": $(jq -Rn --arg v "$NOTES" '$v // empty')
}
JSON

cp "$TMP_JSON" "$OUT_FILE"
rm -f "$TMP_JSON"
chmod 600 "$OUT_FILE" || true

echo "guildnet.config written: $OUT_FILE"
echo "  Host App URL: $HOSTAPP_URL"
if [ -n "$LOGIN_SERVER" ]; then echo "  Login server: $LOGIN_SERVER"; fi
if [ -n "$HOSTNAME_SUGGEST" ]; then echo "  Hostname suggest: $HOSTNAME_SUGGEST"; fi
if [ -n "$NAME_LABEL" ]; then echo "  Name: $NAME_LABEL"; fi
if [ -n "$AUTH_KEY" ]; then echo "  Includes pre-auth key (sensitive)"; fi
