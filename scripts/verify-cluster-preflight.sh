#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"

KUBECONFIG=${KUBECONFIG:-${GN_KUBECONFIG:-${KUBECONFIG:-$HOME/.kube/config}}}
echo "[preflight] Using kubeconfig: $KUBECONFIG"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing required tool: $1" >&2; exit 2; } }
need kubectl

export KUBECONFIG="$KUBECONFIG"

echo "[preflight] Checking API reachability..."
if ! kubectl --request-timeout=5s get --raw='/readyz' >/dev/null 2>&1; then
  echo "[preflight] Kubernetes API not reachable with kubeconfig $KUBECONFIG" >&2
  exit 3
fi

echo "[preflight] Checking RBAC: can create namespaces and CRDs..."
if ! kubectl auth can-i create namespaces >/dev/null 2>&1; then
  echo "[preflight] Current user cannot create namespaces. Ensure kubeconfig has sufficient permissions." >&2
  exit 4
fi
if ! kubectl auth can-i create customresourcedefinitions.apiextensions.k8s.io >/dev/null 2>&1; then
  echo "[preflight] Current user cannot create CRDs. Ensure kubeconfig has cluster-admin or CRD-create permissions." >&2
  exit 4
fi

echo "[preflight] Checking for StorageClass..."
if ! kubectl get storageclass >/dev/null 2>&1; then
  echo "[preflight] No StorageClass detected (or insufficient permissions). Some workloads may require dynamic PV provisioning." >&2
fi

echo "[preflight] Checking LoadBalancer capability / MetalLB..."
# If METALLB_POOL_RANGE is set we assume the operator will deploy MetalLB
if [ -n "${METALLB_POOL_RANGE:-}" ]; then
  echo "[preflight] METALLB_POOL_RANGE set to ${METALLB_POOL_RANGE}; MetalLB pool will be used"
else
  # If metallb namespace present or CRDs present, assume MetalLB
  if kubectl get ns metallb-system >/dev/null 2>&1 || kubectl get crd ipaddresspools.metallb.io >/dev/null 2>&1; then
    echo "[preflight] MetalLB appears to be installed"
  else
    echo "[preflight] No MetalLB detected and METALLB_POOL_RANGE not set. In a real k8s cluster you must provide L2 IPs for LoadBalancer services."
    echo "Either install MetalLB and set METALLB_POOL_RANGE, or provide external LoadBalancer support."
  fi
fi

echo "[preflight] All checks completed (note: some checks may be advisory)."
exit 0
