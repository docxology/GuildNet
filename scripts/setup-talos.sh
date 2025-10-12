#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

# Export default kubeconfig path for consistency across steps
export KUBECONFIG="${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}"

# 1) Preflight (reachability + overlay)
bash "$ROOT/scripts/setup-talos-preflight.sh"

# 2) Generate configs (respect FORCE=1)
bash "$ROOT/scripts/setup-talos-config.sh"

# 3) Reset/apply/bootstrap
bash "$ROOT/scripts/setup-talos-apply.sh"

# 4) Wait for Kubernetes and fetch kubeconfig
bash "$ROOT/scripts/setup-talos-wait-kube.sh"

# 5) Ensure Tailscale subnet router is deployed (provides routes for other tailnet machines)
TS_AUTHKEY=${TS_AUTHKEY:-${HEADSCALE_AUTHKEY:-}}
if [ -n "$TS_AUTHKEY" ]; then
  echo "Ensuring Tailscale subnet router DaemonSet..."
  TS_AUTHKEY="$TS_AUTHKEY" bash "$ROOT/scripts/deploy-tailscale-router.sh" || true
else
  echo "SKIP: TS_AUTHKEY not set; tailscale subnet router not deployed."
fi

echo "Talos setup complete."