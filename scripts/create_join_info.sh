#!/usr/bin/env bash
set -euo pipefail

# Create a portable GuildNet join configuration (guildnet.config)
# This captures everything needed to add a cluster via the UI wizard:
# - Cluster: name label and kubeconfig YAML (required)
# - Optional UI base URL (vite_api_base) and an extra CA PEM to trust it
# - Optional notes for the operator/UI
#
# Backwards-compatible optional fields retained for Tailscale/Headscale hints:
# - tailscale.login_server, tailscale.preauth_key, tailscale.hostname_suggest
#
# The resulting JSON can be safely shared (kubeconfig may contain credentials; handle securely).
#
# Examples:
#   scripts/create_join_info.sh \
#     --kubeconfig ~/.kube/config \
#     --name "Dev Cluster" \
#     --out guildnet.config
#
#   scripts/create_join_info.sh \
#     --kubeconfig ~/.kube/config \
#     --name "Prod EKS" \
#     --hostapp-url https://guildnet.example.com \
#     --include-ca certs/server.crt \
#     --notes "Use SSO account"
#
# Notes:
# - File is written with permissions 0600.
# - jq must be installed (used to safely encode strings as JSON).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Defaults
OUT_FILE="guildnet.config"
HOSTAPP_URL=""
INCLUDE_CA=""
KUBECONFIG_PATH="${KUBECONFIG:-$HOME/.kube/config}"
LOGIN_SERVER=""
AUTH_KEY=""
HOSTNAME_SUGGEST=""
NAME_LABEL=""
NOTES=""

usage() {
  cat <<USAGE
Usage: scripts/create_join_info.sh [options]

Options:
  --kubeconfig PATH     Kubeconfig file to embed (default: \$KUBECONFIG or ~/.kube/config)
  --name LABEL          Optional display name/cluster label shown in UI
  --notes TEXT          Optional free-form note/instructions
  --hostapp-url URL     Optional Host App base URL for UI/API (sets ui.vite_api_base)
  --include-ca PATH     Include PEM-encoded CA/cert at PATH (for ui.ca_pem)
  --login-server URL    (optional) Tailscale/Headscale login server URL (hint)
  --auth-key KEY        (optional) Pre-auth key (sensitive, hint)
  --hostname NAME       (optional) Suggested tsnet hostname (hint)
  --out FILE            Output path (default: guildnet.config)
  --help                Show this help
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --kubeconfig) KUBECONFIG_PATH="${2:-}"; shift 2 ;;
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

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq is required (for JSON encoding)" >&2
  exit 2
fi

# Read kubeconfig
if [ -z "$KUBECONFIG_PATH" ] || [ ! -s "$KUBECONFIG_PATH" ]; then
  echo "ERROR: --kubeconfig PATH is required and must exist (got: '$KUBECONFIG_PATH')" >&2
  exit 2
fi
KC_YAML="$(cat "$KUBECONFIG_PATH")"
KC_JSON="$(jq -Rs . <<<"$KC_YAML")"

# Read CA PEM if requested or fallback to repo certs/server.crt (optional)
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

# Build dynamic UI object fields (conditionally emit keys)
UI_FIELDS=()
if [ -n "$HOSTAPP_URL" ]; then UI_FIELDS+=("\"vite_api_base\": $(jq -Rn --arg v \"$HOSTAPP_URL\" '$v')"); fi
if [ -n "$CA_PEM" ]; then UI_FIELDS+=("\"ca_pem\": $(jq -Rs . <<<\"$CA_PEM\")"); fi
UI_JSON_CONTENT="$(IFS=, ; echo "${UI_FIELDS[*]}")"

# Metadata
NOW_ISO="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
HOSTNAME_LOCAL="$(hostname 2>/dev/null || echo unknown)"
CREATOR_USER="${USER:-unknown}"

TMP_JSON="$(mktemp)"
cat >"$TMP_JSON" <<JSON
{
  "version": 2,
  "created_at": "$NOW_ISO",
  "creator": {"host": "$HOSTNAME_LOCAL", "user": "$CREATOR_USER"},
  "ui": { $UI_JSON_CONTENT },
  "cluster": {
    "name": $(jq -Rn --arg v "$NAME_LABEL" '$v // empty'),
    "kubeconfig": $KC_JSON,
    "notes": $(jq -Rn --arg v "$NOTES" '$v // empty')
  },
  "tailscale": {
    "login_server": $(jq -Rn --arg v "$LOGIN_SERVER" '$v // empty'),
    "preauth_key": $(jq -Rn --arg v "$AUTH_KEY" '$v // empty'),
    "hostname_suggest": $(jq -Rn --arg v "$HOSTNAME_SUGGEST" '$v // empty')
  }
}
JSON

cp "$TMP_JSON" "$OUT_FILE"
rm -f "$TMP_JSON"
chmod 600 "$OUT_FILE" || true

# Summary
echo "guildnet.config written: $OUT_FILE"
if [ -n "$NAME_LABEL" ]; then echo "  Name: $NAME_LABEL"; fi
echo "  Kubeconfig: $KUBECONFIG_PATH"
if [ -n "$HOSTAPP_URL" ]; then echo "  UI base: $HOSTAPP_URL"; fi
if [ -n "$LOGIN_SERVER" ]; then echo "  Login server: $LOGIN_SERVER"; fi
if [ -n "$HOSTNAME_SUGGEST" ]; then echo "  Hostname suggest: $HOSTNAME_SUGGEST"; fi
if [ -n "$AUTH_KEY" ]; then echo "  Includes pre-auth key (sensitive)"; fi
