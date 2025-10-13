#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"
export KUBECONFIG="${KUBECONFIG:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need kubectl
need jq

PASS=1

echo "--- Headscale reachability ---"
HS=${HEADSCALE_URL:-}
if [ -z "$HS" ] && docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$'; then
  HOST=$(docker inspect -f '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostIp }}' guildnet-headscale 2>/dev/null || echo 127.0.0.1)
  PORT=$(docker inspect -f '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort }}' guildnet-headscale 2>/dev/null || echo 8081)
  [ "$HOST" = "0.0.0.0" ] && HOST=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}' | head -n1)
  HS="http://${HOST}:${PORT}"
fi
if [ -n "$HS" ]; then
  if curl -fsS "$HS" >/dev/null; then echo ok; else echo fail; PASS=0; fi
else
  echo "skip (no headscale)"
fi

# Router status
bash "$ROOT/scripts/tailscale-router.sh" status || true

# Headscale routes
if docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$'; then
  docker exec -i guildnet-headscale headscale routes list || true
fi

# Router DS readiness
echo "--- Tailscale router DaemonSet ---"
if kubectl -n kube-system get ds tailscale-subnet-router >/dev/null 2>&1; then
  if kubectl -n kube-system rollout status ds/tailscale-subnet-router --timeout=60s; then echo ok; else PASS=0; fi
else
  echo "not found"; PASS=0
fi

# Kube readyz + nodes
kubectl --request-timeout=5s get --raw='/readyz?verbose' || { PASS=0; true; }
kubectl get nodes -o wide || true

# MetalLB sanity: CRDs + controller/speaker
kubectl get crd ipaddresspools.metallb.io l2advertisements.metallb.io 2>/dev/null || true
kubectl -n metallb-system get deploy controller speaker -o wide 2>/dev/null || true
kubectl -n metallb-system rollout status deploy/controller --timeout=60s || true
kubectl -n metallb-system rollout status deploy/speaker --timeout=60s || true

# DB service
bash "$ROOT/scripts/rethinkdb-setup.sh" || true

echo "verify-e2e completed."
if [ "$PASS" = "1" ]; then
  echo "SUMMARY: PASS"
else
  echo "SUMMARY: FAIL"; exit 1
fi
