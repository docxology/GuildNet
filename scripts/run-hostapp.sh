#!/usr/bin/env bash
set -euo pipefail

# Load env file if present
set -a
if [ -f ".env" ]; then . ./.env; fi
set +a

# Sanity-check TS_LOGIN_SERVER; if it's an invalid host like a bare IP missing scheme or malformed, default to local headscale if running or ask to run setup
if [[ -n "${TS_LOGIN_SERVER:-}" ]]; then
  if ! [[ "$TS_LOGIN_SERVER" =~ ^https?:// ]]; then
    # auto-fix by assuming http:// for bare host/IP
    TS_LOGIN_SERVER="http://${TS_LOGIN_SERVER}"
    export TS_LOGIN_SERVER
  fi
fi

# Resolve kubeconfig
GN_KUBECONFIG_DEFAULT="${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}"
if [ -z "${KUBECONFIG:-}" ]; then
  export KUBECONFIG="$GN_KUBECONFIG_DEFAULT"
fi

# Default listen address may be overridden by caller
LISTEN_LOCAL="${LISTEN_LOCAL:-127.0.0.1:8080}"
export LISTEN_LOCAL

# Prefer using kubectl proxy for API server proxying unless explicitly overridden
if [ -z "${HOSTAPP_API_PROXY_URL:-}" ]; then
  if command -v kubectl >/dev/null 2>&1; then
    # Start kubectl proxy on 127.0.0.1:8001 if not already listening
    if ! (curl -sS --max-time 1 http://127.0.0.1:8001/version >/dev/null 2>&1); then
      ( KUBECONFIG="${KUBECONFIG}" nohup kubectl proxy --port=8001 >/dev/null 2>&1 & ) || true
      # wait briefly for it to come up
      for i in {1..20}; do
        if curl -sS --max-time 1 http://127.0.0.1:8001/version >/dev/null 2>&1; then break; fi
        sleep 0.3
      done
    fi
    export HOSTAPP_API_PROXY_URL="http://127.0.0.1:8001"
    export HOSTAPP_API_PROXY_FORCE_HTTP="1"
  fi
fi

# Database discovery hints (let the server resolve via Kubernetes API using kubeconfig)
: "${RETHINKDB_SERVICE_NAME:=rethinkdb}"
# Allow RETHINKDB_NAMESPACE to be set via .env or fall back to K8S_NAMESPACE or "default"
RETHINKDB_NAMESPACE="${RETHINKDB_NAMESPACE:-${K8S_NAMESPACE:-default}}"
export RETHINKDB_SERVICE_NAME RETHINKDB_NAMESPACE

# Force reliable IDE access via local port-forward fallback unless user overrides
: "${HOSTAPP_USE_PORT_FORWARD:=1}"
export HOSTAPP_USE_PORT_FORWARD

# Ensure the embedded operator runs so Workspaces reconcile locally
: "${HOSTAPP_EMBED_OPERATOR:=1}"
export HOSTAPP_EMBED_OPERATOR

# Run hostapp
exec ./bin/hostapp serve
