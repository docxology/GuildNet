#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"
export KUBECONFIG="${KUBECONFIG:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}"

POOL_NAME=${METALLB_POOL_NAME:-workspaces}
POOL_RANGE=${METALLB_POOL_RANGE:-10.0.0.200-10.0.0.250}

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need kubectl

# Install MetalLB manifests (idempotent)
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.12/config/manifests/metallb-native.yaml >/dev/null || true

# Wait for CRDs to be established before using them (avoid races)
kubectl wait --for=condition=Established crd/ipaddresspools.metallb.io --timeout=180s || true
kubectl wait --for=condition=Established crd/l2advertisements.metallb.io --timeout=180s || true

# Wait for controller/speaker rollouts to complete (best-effort)
kubectl -n metallb-system rollout status deploy/controller --timeout=180s || true
kubectl -n metallb-system rollout status deploy/speaker --timeout=180s || true

# Prepare pool + L2Advertisement manifest
MANIFEST=$(cat <<YAML
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: ${POOL_NAME}
  namespace: metallb-system
spec:
  addresses:
    - ${POOL_RANGE}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: ${POOL_NAME}-l2
  namespace: metallb-system
spec:
  ipAddressPools:
    - ${POOL_NAME}
YAML
)

# Retry a server-side dry-run until the webhook is ready to accept the resource
tries=${METALLB_APPLY_RETRIES:-60}
delay=${METALLB_APPLY_DELAY:-3}
while (( tries > 0 )); do
  if printf '%s' "$MANIFEST" | kubectl apply --dry-run=server -f - >/dev/null 2>&1; then
    break
  fi
  tries=$((tries-1))
  sleep "$delay"
done

# Apply pool + L2Advertisement for real
printf '%s' "$MANIFEST" | kubectl apply -f -

echo "MetalLB installed with pool ${POOL_NAME}=${POOL_RANGE}"
