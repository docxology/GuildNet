#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

# Load environment overrides if present
if [ -f ./.env ]; then
  # shellcheck disable=SC1090
  . ./.env
fi

# 1) Ensure forwarding (may prompt for sudo)
bash "$ROOT/scripts/enable-ip-forwarding.sh" || true

# 1.5) Ensure tailscaled is running and grant operator (best effort)
bash "$ROOT/scripts/bootstrap-sudo.sh" || true

# 2) Install and bring up host tailscale router
echo "-> Installing tailscale client (if missing)"
make router-install || true

echo "-> Ensuring tailscaled daemon is running"
make router-daemon || true

echo "-> Bringing up router (tailscale up)"
make router-up || { echo "router-up failed" >&2; exit 1; }

echo "-> Router status"
make router-status || true

# Wait a moment for tailscale to converge and routes to be applied
sleep 2

# Print status and verify the advertised routes are present locally
if command -v tailscale >/dev/null 2>&1; then
  echo "Local tailscale status summary:"
  tailscale status --json | jq -r '.Self|{PrimaryRoutes:.PrimaryRoutes,AllowedIPs:.AllowedIPs}' || tailscale status || true
fi

# 3) Approve routes in Headscale (best-effort)
if docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$'; then
  echo "-> Approving routes in Headscale (if any)"
  bash "$ROOT/scripts/headscale-approve-routes.sh" || true
fi

echo "Tailscale router setup complete."

# 4) Ensure there's an advertiser for the cluster CIDR (10.0.0.0/24)
# If the cluster kube API is reachable, deploy the in-cluster tailscale subnet router DaemonSet.
# Otherwise, try to bring up a local overlay (Headscale + router) if available on this host.
echo "-> Ensuring a subnet-router/advertiser is present for TS_ROUTES"
KUBECTL_OK=1
if command -v kubectl >/dev/null 2>&1; then
  if KUBECONFIG=${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig} kubectl version --short >/dev/null 2>&1; then
    KUBECTL_OK=0
  fi
fi

if [ "$KUBECTL_OK" -eq 0 ]; then
  echo "Cluster API reachable via KUBECONFIG; deploying in-cluster subnet router (router-ensure)"
  make router-ensure || { echo "router-ensure failed; you can try: make router-ensure-novalidate" >&2; }
else
  echo "Cluster API not reachable from this host. Checking for a local Headscale container..."
  # If SSH tunnel is configured, attempt to open it and use it to reach the API
  if [ -n "${SSH_TUNNEL_HOST:-}" ] && [ -n "${SSH_TUNNEL_USER:-}" ]; then
    echo "SSH_TUNNEL configured: attempting to open tunnel to ${SSH_TUNNEL_USER}@${SSH_TUNNEL_HOST} -> ${REMOTE_IP:-10.0.0.10}:6443"
    LOCAL_PORT=${LOCAL_PORT:-16443}
    REMOTE_IP=${REMOTE_IP:-10.0.0.10}
    # Start tunnel
    SSH_TUNNEL_HOST=${SSH_TUNNEL_HOST} SSH_TUNNEL_USER=${SSH_TUNNEL_USER} REMOTE_IP=${REMOTE_IP} LOCAL_PORT=${LOCAL_PORT} bash ./scripts/tunnel-kubeapi.sh start || true
    # Create a temporary kubeconfig with server rewritten to point to localhost:LOCAL_PORT
    if [ -f "${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}" ]; then
      TMP_KUBECONFIG=$(mktemp)
      sed -E "s@(server: https?://)[^:]+:([0-9]+)@\1127.0.0.1:${LOCAL_PORT}@g" "${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}" > "$TMP_KUBECONFIG" || true
      # Try kubectl with the temporary kubeconfig
      if KUBECONFIG="$TMP_KUBECONFIG" kubectl version >/dev/null 2>&1; then
        echo "Tunnel appears to provide API reachability; deploying router-ensure via forwarded port"
        # Use router-ensure-novalidate to avoid server-side validation if API requires different hostnames
        KUBECONFIG="$TMP_KUBECONFIG" make router-ensure-novalidate || echo "router-ensure-novalidate failed" >&2
      else
        echo "Tunnel did not expose the kube API successfully; stopping tunnel"
        bash ./scripts/tunnel-kubeapi.sh stop || true
      fi
      rm -f "$TMP_KUBECONFIG" || true
    else
      echo "No GN_KUBECONFIG file present at ${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}; cannot try tunnel-based router-ensure"
      bash ./scripts/tunnel-kubeapi.sh stop || true
    fi
  fi
  if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$$'; then
    echo "Local Headscale detected; bringing up local overlay (headscale + router)"
    make local-overlay-up || echo "local-overlay-up failed; please inspect headscale and router logs" >&2
  else
    echo "No local Headscale found and cluster API unreachable."
    echo "Please ensure the node that advertises ${TS_ROUTES%%,*} is online in the tailnet or run the subnet router inside the cluster."
    echo "If that node is remote, bring it online or run: make headscale-up (on the host that should host Headscale) and then make router-up there."
  fi
fi