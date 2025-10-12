#!/usr/bin/env bash
set -euo pipefail

# Create a portable GuildNet join configuration (guildnet.config)
# This captures everything needed to add a cluster via the UI wizard:
# - Cluster: name label and kubeconfig YAML (optional; include if available)
# - Optional UI base URL (vite_api_base) and an extra CA PEM to trust it
# - Optional hostapp.url and hostapp.ca_pem for CLI join compatibility
# - Optional notes for the operator/UI
# - Optional per-cluster connectivity/proxy/ingress knobs
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
#     --namespace dev \
#     --api-proxy-url http://127.0.0.1:8001 \
#     --prefer-pod-proxy \
#     --use-port-forward \
#     --ingress-domain work.example.com \
#     --ingress-class-name nginx \
#     --workspace-tls-secret wildcard-tls \
#     --cert-manager-issuer letsencrypt \
#     --out guildnet.config
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
# Per-cluster settings (optional)
NAMESPACE=""
API_PROXY_URL=""
API_PROXY_FORCE_HTTP="0"
DISABLE_API_PROXY="0"
PREFER_POD_PROXY="0"
USE_PORT_FORWARD="0"
INGRESS_DOMAIN=""
INGRESS_CLASS_NAME=""
WORKSPACE_TLS_SECRET=""
CERT_MANAGER_ISSUER=""
INGRESS_AUTH_URL=""
INGRESS_AUTH_SIGNIN=""
IMAGE_PULL_SECRET=""
ORG_ID=""

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
  --namespace NAME      (optional) Default Kubernetes namespace for GuildNet resources
  --api-proxy-url URL   (optional) Override base URL for Kubernetes API (e.g., http://127.0.0.1:8001)
  --api-proxy-force-http   (flag) Force HTTP scheme for API proxy
  --disable-api-proxy      (flag) Disable API proxy usage for this cluster
  --prefer-pod-proxy       (flag) Prefer pod proxy over service proxy for workload access
  --use-port-forward       (flag) Try local kubectl port-forward before API proxy
  --ingress-domain DOMAIN  (optional) Base domain for per-workspace ingress
  --ingress-class-name CLS (optional) IngressClass name (e.g., nginx)
  --workspace-tls-secret S (optional) TLS secret name for workspace hosts
  --cert-manager-issuer I  (optional) cert-manager ClusterIssuer for TLS
  --ingress-auth-url URL   (optional) NGINX auth-url annotation
  --ingress-auth-signin U  (optional) NGINX auth-signin annotation
  --image-pull-secret S    (optional) ImagePullSecret name for workloads
  --org-id ID              (optional) Organization/tenant id associated to this cluster
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
    --namespace) NAMESPACE="${2:-}"; shift 2 ;;
    --api-proxy-url) API_PROXY_URL="${2:-}"; shift 2 ;;
    --api-proxy-force-http) API_PROXY_FORCE_HTTP="1"; shift 1 ;;
    --disable-api-proxy) DISABLE_API_PROXY="1"; shift 1 ;;
    --prefer-pod-proxy) PREFER_POD_PROXY="1"; shift 1 ;;
    --use-port-forward) USE_PORT_FORWARD="1"; shift 1 ;;
    --ingress-domain) INGRESS_DOMAIN="${2:-}"; shift 2 ;;
    --ingress-class-name) INGRESS_CLASS_NAME="${2:-}"; shift 2 ;;
    --workspace-tls-secret) WORKSPACE_TLS_SECRET="${2:-}"; shift 2 ;;
    --cert-manager-issuer) CERT_MANAGER_ISSUER="${2:-}"; shift 2 ;;
    --ingress-auth-url) INGRESS_AUTH_URL="${2:-}"; shift 2 ;;
    --ingress-auth-signin) INGRESS_AUTH_SIGNIN="${2:-}"; shift 2 ;;
    --image-pull-secret) IMAGE_PULL_SECRET="${2:-}"; shift 2 ;;
    --org-id) ORG_ID="${2:-}"; shift 2 ;;
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
NAMESPACE="$(strip_quotes "$NAMESPACE")"
API_PROXY_URL="$(strip_quotes "$API_PROXY_URL")"
INGRESS_DOMAIN="$(strip_quotes "$INGRESS_DOMAIN")"
INGRESS_CLASS_NAME="$(strip_quotes "$INGRESS_CLASS_NAME")"
WORKSPACE_TLS_SECRET="$(strip_quotes "$WORKSPACE_TLS_SECRET")"
CERT_MANAGER_ISSUER="$(strip_quotes "$CERT_MANAGER_ISSUER")"
INGRESS_AUTH_URL="$(strip_quotes "$INGRESS_AUTH_URL")"
INGRESS_AUTH_SIGNIN="$(strip_quotes "$INGRESS_AUTH_SIGNIN")"
IMAGE_PULL_SECRET="$(strip_quotes "$IMAGE_PULL_SECRET")"
ORG_ID="$(strip_quotes "$ORG_ID")"

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
  --arg ns "$NAMESPACE" \
  --arg api_proxy "$API_PROXY_URL" \
  --arg api_force "$API_PROXY_FORCE_HTTP" \
  --arg disable_proxy "$DISABLE_API_PROXY" \
  --arg prefer_pod "$PREFER_POD_PROXY" \
  --arg use_pf "$USE_PORT_FORWARD" \
  --arg dom "$INGRESS_DOMAIN" \
  --arg iclass "$INGRESS_CLASS_NAME" \
  --arg tlssec "$WORKSPACE_TLS_SECRET" \
  --arg issuer "$CERT_MANAGER_ISSUER" \
  --arg authurl "$INGRESS_AUTH_URL" \
  --arg authsignin "$INGRESS_AUTH_SIGNIN" \
  --arg imgsec "$IMAGE_PULL_SECRET" \
  --arg org "$ORG_ID" \
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
     cluster: (
       {} 
       | . + { name: ($name // ""), kubeconfig: $kc, notes: ($notes // "") }
       | (if ($ns|length)>0 then . + { namespace: $ns } else . end)
       | (if ($api_proxy|length)>0 then . + { api_proxy_url: $api_proxy } else . end)
       | (if ($api_force|length)>0 and ($api_force!="0") then . + { api_proxy_force_http: true } else . end)
       | (if ($disable_proxy|length)>0 and ($disable_proxy!="0") then . + { disable_api_proxy: true } else . end)
       | (if ($prefer_pod|length)>0 and ($prefer_pod!="0") then . + { prefer_pod_proxy: true } else . end)
       | (if ($use_pf|length)>0 and ($use_pf!="0") then . + { use_port_forward: true } else . end)
       | (if ($dom|length)>0 then . + { ingress_domain: $dom } else . end)
       | (if ($iclass|length)>0 then . + { ingress_class_name: $iclass } else . end)
       | (if ($tlssec|length)>0 then . + { workspace_tls_secret: $tlssec } else . end)
       | (if ($issuer|length)>0 then . + { cert_manager_issuer: $issuer } else . end)
       | (if ($authurl|length)>0 then . + { ingress_auth_url: $authurl } else . end)
       | (if ($authsignin|length)>0 then . + { ingress_auth_signin: $authsignin } else . end)
       | (if ($imgsec|length)>0 then . + { image_pull_secret: $imgsec } else . end)
       | (if ($org|length)>0 then . + { org_id: $org } else . end)
     ),
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
