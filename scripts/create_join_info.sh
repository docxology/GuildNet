#!/usr/bin/env bash
set -euo pipefail

# Create a portable GuildNet join configuration (guildnet.config)
# This captures everything needed to add a cluster via the UI wizard:
# - Cluster: name label and kubeconfig YAML (optional; include if available)
# - Optional UI base URL (vite_api_base) and an extra CA PEM to trust it
# - Optional hostapp.url and hostapp.ca_pem for CLI join compatibility
# - Optional notes for the operator/UI
#
# Optional fields retained for Tailscale/Headscale hints:
# - tailscale.login_server, tailscale.preauth_key, tailscale.hostname
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
HOSTNAME=""
NAME_LABEL=""
NOTES=""

usage() {
  cat <<USAGE
Usage: scripts/create_join_info.sh [options]

Options:
  --kubeconfig PATH     Kubeconfig file to embed (defaults: KUBECONFIG, ~/.kube/config; falls back to ~/.guildnet/kubeconfig)
  --name LABEL          Optional display name/cluster label shown in UI
  --notes TEXT          Optional free-form note/instructions
  --hostapp-url URL     Optional Host App base URL for UI/API (sets ui.vite_api_base and hostapp.url)
  --include-ca PATH     Include PEM-encoded CA/cert at PATH (for ui.ca_pem and hostapp.ca_pem)
  --login-server URL    (optional) Tailscale/Headscale login server URL (hint)
  --auth-key KEY        (optional) Pre-auth key (sensitive, hint)
  --hostname NAME       (optional) tsnet hostname (hint)
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
    --hostname) HOSTNAME="${2:-}"; shift 2 ;;
    --name) NAME_LABEL="${2:-}"; shift 2 ;;
    --notes) NOTES="${2:-}"; shift 2 ;;
    --out) OUT_FILE="${2:-}"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown arg: $1" >&2; usage; exit 2 ;;
  esac
done

# Sanitize: strip wrapping quotes if present
strip_quotes() { local s="$1"; s="${s%\"}"; s="${s#\"}"; printf '%s' "$s"; }
HOSTAPP_URL="$(strip_quotes "$HOSTAPP_URL")"
LOGIN_SERVER="$(strip_quotes "$LOGIN_SERVER")"
HOSTNAME="$(strip_quotes "$HOSTNAME")"
NAME_LABEL="$(strip_quotes "$NAME_LABEL")"
NOTES="$(strip_quotes "$NOTES")"

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq is required (for JSON encoding)" >&2
  exit 2
fi

# Fallbacks from local GuildNet config if available
LOCAL_CFG="$HOME/.guildnet/config.json"
if [ -z "$LOGIN_SERVER" ] && [ -s "$LOCAL_CFG" ]; then
  LOGIN_SERVER="$(jq -r '.login_server // empty' "$LOCAL_CFG" 2>/dev/null || true)"
fi
if [ -z "$AUTH_KEY" ] && [ -s "$LOCAL_CFG" ]; then
  AUTH_KEY="$(jq -r '.auth_key // empty' "$LOCAL_CFG" 2>/dev/null || true)"
fi
if [ -z "$HOSTNAME" ]; then
  HOSTNAME="$(hostname 2>/dev/null || echo "")"
fi

# Read kubeconfig (optional; try fallbacks)
KC_YAML=""
_pick_kc() {
  # prefer explicit
  if [ -n "${KUBECONFIG_PATH:-}" ] && [ -s "${KUBECONFIG_PATH:-}" ]; then echo "$KUBECONFIG_PATH"; return; fi
  # common fallbacks
  if [ -s "$HOME/.guildnet/kubeconfig" ]; then echo "$HOME/.guildnet/kubeconfig"; return; fi
  if [ -s "$HOME/.kube/config" ]; then echo "$HOME/.kube/config"; return; fi
  # none
  echo ""; return
}
KC_PATH="$(_pick_kc)"
if [ -n "$KC_PATH" ]; then
  KC_YAML="$(cat "$KC_PATH")"
else
  echo "WARN: no kubeconfig found; writing empty cluster.kubeconfig" >&2
fi

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

# Metadata
NOW_ISO="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
HOSTNAME_LOCAL="$(hostname 2>/dev/null || echo unknown)"
CREATOR_USER="${USER:-unknown}"

# Build JSON via jq to avoid quoting issues
jq -n \
  --arg now "$NOW_ISO" \
  --arg host "$HOSTNAME_LOCAL" \
  --arg user "$CREATOR_USER" \
  --arg ui_url "$HOSTAPP_URL" \
  --arg ca_pem "$CA_PEM" \
  --arg hostapp_url "$HOSTAPP_URL" \
  --arg kc "$KC_YAML" \
  --arg name "$NAME_LABEL" \
  --arg notes "$NOTES" \
  --arg login "$LOGIN_SERVER" \
  --arg auth "$AUTH_KEY" \
  --arg tsname "$HOSTNAME" \
  '{
     version: 2,
     created_at: $now,
     creator: {host: $host, user: $user},
     ui: ({} 
          | (if ($ui_url|length)>0 then . + {vite_api_base: $ui_url} else . end)
          | (if ($ca_pem|length)>0 then . + {ca_pem: $ca_pem} else . end)),
     hostapp: ({} 
          | (if ($hostapp_url|length)>0 then . + {url: $hostapp_url} else . end)
          | (if ($ca_pem|length)>0 then . + {ca_pem: $ca_pem} else . end)),
     cluster: {
       name: ($name // ""),
       kubeconfig: $kc,
       notes: ($notes // "")
     },
     tailscale: {
       login_server: ($login // ""),
       preauth_key: ($auth // ""),
       hostname: ($tsname // "")
     }
   }' > "$OUT_FILE"

chmod 600 "$OUT_FILE" || true

# Summary
echo "guildnet.config written: $OUT_FILE"
if [ -n "$NAME_LABEL" ]; then echo "  Name: $NAME_LABEL"; fi
if [ -n "$KC_PATH" ]; then echo "  Kubeconfig: $KC_PATH"; else echo "  Kubeconfig: (none)"; fi
if [ -n "$HOSTAPP_URL" ]; then echo "  UI base: $HOSTAPP_URL"; fi
if [ -n "$LOGIN_SERVER" ]; then echo "  Login server: $LOGIN_SERVER"; fi
if [ -n "$HOSTNAME" ]; then echo "  Hostname: $HOSTNAME"; fi
if [ -n "$AUTH_KEY" ]; then echo "  Includes pre-auth key (sensitive)"; fi
