#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"
export KUBECONFIG="${KUBECONFIG:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need kubectl

# Router status
bash "$ROOT/scripts/tailscale-router.sh" status || true

# Headscale routes
if docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$'; then
  docker exec -i guildnet-headscale headscale routes list || true
fi

# Kube readyz + nodes
kubectl --request-timeout=5s get --raw='/readyz?verbose' || true
kubectl get nodes -o wide || true

# MetalLB sanity: CRDs + controller/speaker
kubectl get crd ipaddresspools.metallb.io l2advertisements.metallb.io 2>/dev/null || true
kubectl -n metallb-system get deploy controller speaker -o wide 2>/dev/null || true
kubectl -n metallb-system rollout status deploy/controller --timeout=60s || true
kubectl -n metallb-system rollout status deploy/speaker --timeout=60s || true

# DB service
bash "$ROOT/scripts/rethinkdb-setup.sh" || true

echo "verify-e2e completed."
