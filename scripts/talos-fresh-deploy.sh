#!/usr/bin/env bash
# Fresh Talos cluster deployment helper (wipe & recreate).
# Assumes talosctl is installed and accessible.
#
# Usage:
#   scripts/talos-fresh-deploy.sh \
#     --cluster mycluster \
#     --endpoint https://<control-plane-endpoint>:6443 \
#     --cp <cp1-ip,cp2-ip,...> \
#     --workers <w1-ip,w2-ip,...>
#
set -euo pipefail

CLUSTER="mycluster"
ENDPOINT=""
CP_NODES=""
WK_NODES=""
OUT_DIR="./talos"
FORCE=0
declare -a WK_ARR=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster) CLUSTER="$2"; shift 2 ;;
    --endpoint) ENDPOINT="$2"; shift 2 ;;
    --cp) CP_NODES="$2"; shift 2 ;;
    --workers) WK_NODES="$2"; shift 2 ;;
    --out) OUT_DIR="$2"; shift 2 ;;
    --force) FORCE=1; shift 1 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$ENDPOINT" || -z "$CP_NODES" ]]; then
  echo "--endpoint and --cp are required" >&2
  exit 2
fi

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need talosctl

IFS=',' read -r -a CP_ARR <<< "$CP_NODES"
# Allow empty workers gracefully; if WK_NODES is empty or unset after read, define empty array
if [[ -n "${WK_NODES}" ]]; then
  IFS=',' read -r -a WK_ARR <<< "$WK_NODES"
else
  WK_ARR=()
fi

mkdir -p "$OUT_DIR"

echo "[1/7] Generating cluster config..."
if [[ $FORCE -eq 1 ]]; then
  echo "  --force specified: regenerating config into $OUT_DIR" >&2
  talosctl gen config "$CLUSTER" "$ENDPOINT" --output-dir "$OUT_DIR" --force
else
  if [[ -f "$OUT_DIR/controlplane.yaml" ]]; then
    echo "  existing config detected (use --force to regenerate); skipping generation"
  else
    talosctl gen config "$CLUSTER" "$ENDPOINT" --output-dir "$OUT_DIR"
  fi
fi

echo "[2/7] Resetting any existing nodes (if reachable)..."
for n in "${CP_ARR[@]}"; do
  echo "  resetting control-plane $n"
  talosctl reset --nodes "$n" --reboot --graceful=false || true
done
for n in "${WK_ARR[@]}"; do
  echo "  resetting worker $n"
  talosctl reset --nodes "$n" --reboot --graceful=false || true
done

echo "[3/7] Waiting for nodes to become reachable (post-reset) ..."
wait_node() {
  local node=$1; local tries=60; local delay=5
  while (( tries > 0 )); do
    if talosctl version --nodes "$node" >/dev/null 2>&1; then
      echo "    node $node is reachable"
      return 0
    fi
    ((tries--))
    sleep "$delay"
  done
  echo "WARNING: node $node not reachable after wait" >&2
  return 1
}
for n in "${CP_ARR[@]}"; do wait_node "$n" || true; done
for n in "${WK_ARR[@]}"; do wait_node "$n" || true; done

echo "[4/7] Applying control-plane configs..."
for n in "${CP_ARR[@]}"; do
  echo "  apply config to control-plane $n"
  talosctl apply-config --insecure --nodes "$n" --file "$OUT_DIR/controlplane.yaml"
done

echo "[5/7] Bootstrapping etcd on first CP node (idempotent)..."
if ! talosctl get etcdmember --nodes "${CP_ARR[0]}" >/dev/null 2>&1; then
  talosctl --nodes "${CP_ARR[0]}" bootstrap || {
    echo "Bootstrap attempt failed; will still proceed (may already be bootstrapped)" >&2
  }
else
  echo "  etcd appears bootstrapped already"
fi

if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  echo "[6/7] Applying worker configs..."
  for n in "${WK_ARR[@]}"; do
    echo "  apply config to worker $n"
    talosctl apply-config --insecure --nodes "$n" --file "$OUT_DIR/worker.yaml"
  done
else
  echo "[6/7] No worker nodes specified, skipping"
fi

echo "[7/7] Waiting for Kubernetes API (kubelet nodes Ready)..."
wait_kube() {
  local tries=90; local delay=5
  while (( tries > 0 )); do
    if kubectl get nodes >/dev/null 2>&1; then
      kubectl get nodes -o wide || true
      return 0
    fi
    ((tries--))
    sleep "$delay"
  done
  echo "WARNING: Kubernetes API not ready after wait" >&2
  return 1
}
wait_kube || true

echo "Fetching kubeconfig..."
talosctl kubeconfig --nodes "${CP_ARR[0]}" --force

echo "Done. Verify with: kubectl get nodes -o wide"
