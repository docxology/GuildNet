#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"
# Default dev values for per-workspace ingress
export WORKSPACE_DOMAIN=${WORKSPACE_DOMAIN:-workspaces.127.0.0.1.nip.io}
export INGRESS_CLASS_NAME=${INGRESS_CLASS_NAME:-nginx}
# For dev with cert-manager staging cluster issuer, uncomment:
# export CERT_MANAGER_ISSUER=${CERT_MANAGER_ISSUER:-letsencrypt-staging}
# If you already have a wildcard secret, set it here; otherwise per-host certs will be requested when CERT_MANAGER_ISSUER is set
export WORKSPACE_TLS_SECRET=${WORKSPACE_TLS_SECRET:-}
# Optional oauth2-proxy endpoints (set if deployed)
export INGRESS_AUTH_URL=${INGRESS_AUTH_URL:-}
export INGRESS_AUTH_SIGNIN=${INGRESS_AUTH_SIGNIN:-}
echo "Configured:"
env | grep -E 'WORKSPACE_DOMAIN|INGRESS_CLASS_NAME|CERT_MANAGER_ISSUER|WORKSPACE_TLS_SECRET|INGRESS_AUTH_' || true