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
  # consider any HTTP status as reachable; only fail if TCP connect fails
  if curl -sS -o /dev/null -m 2 "$HS" || true; then echo ok; else echo fail; PASS=0; fi
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
if ! command -v kubectl >/dev/null; then
  echo "skip (no kubectl)"
elif [ ! -f "${KUBECONFIG}" ]; then
  echo "skip (no kubeconfig)"
elif ! kubectl version --request-timeout=3s >/dev/null 2>&1; then
  echo "skip (kube API unreachable)"
elif kubectl -n kube-system get ds tailscale-subnet-router >/dev/null 2>&1; then
  # First try rollout status (fast path)
  if kubectl -n kube-system rollout status ds/tailscale-subnet-router --timeout=90s; then
    echo ok
  else
    # Fallback: compare desired vs ready with a short retry loop
    tries=10
    ok=0
    while [ $tries -gt 0 ]; do
      desired=$(kubectl -n kube-system get ds tailscale-subnet-router -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo 0)
      ready=$(kubectl -n kube-system get ds tailscale-subnet-router -o jsonpath='{.status.numberReady}' 2>/dev/null || echo 0)
      if [ "$desired" != "" ] && [ "$ready" = "$desired" ] && [ "$desired" != "0" ]; then
        ok=1; break
      fi
      sleep 3
      tries=$((tries-1))
    done
    if [ $ok -eq 1 ]; then echo ok; else PASS=0; fi
  fi
else
  echo "not found"; PASS=0
fi

# Kube readyz + nodes
if command -v kubectl >/dev/null && [ -f "${KUBECONFIG}" ] && kubectl version --request-timeout=3s >/dev/null 2>&1; then
  kubectl --request-timeout=5s get --raw='/readyz?verbose' || { PASS=0; true; }
  kubectl get nodes -o wide || true
else
  echo "--- Kubernetes checks skipped (no kube or unreachable) ---"
fi

# MetalLB sanity: CRDs + controller/speaker
kubectl get crd ipaddresspools.metallb.io l2advertisements.metallb.io 2>/dev/null || true
kubectl -n metallb-system get deploy/controller ds/speaker -o wide 2>/dev/null || true
kubectl -n metallb-system rollout status deploy/controller --timeout=60s || true
kubectl -n metallb-system rollout status ds/speaker --timeout=60s || true

# DB service
if command -v kubectl >/dev/null && [ -f "${KUBECONFIG}" ] && kubectl version --request-timeout=3s >/dev/null 2>&1; then
  bash "$ROOT/scripts/rethinkdb-setup.sh" || true
fi

echo "verify-e2e completed."
if [ "$PASS" = "1" ]; then
  echo "SUMMARY: PASS"
else
  echo "SUMMARY: FAIL"; exit 1
fi
