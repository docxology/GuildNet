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

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster) CLUSTER="$2"; shift 2 ;;
    --endpoint) ENDPOINT="$2"; shift 2 ;;
    --cp) CP_NODES="$2"; shift 2 ;;
    --workers) WK_NODES="$2"; shift 2 ;;
    --out) OUT_DIR="$2"; shift 2 ;;
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
IFS=',' read -r -a WK_ARR <<< "$WK_NODES"

mkdir -p "$OUT_DIR"

echo "[1/5] Generating cluster config..."
talosctl gen config "$CLUSTER" "$ENDPOINT" --output-dir "$OUT_DIR"

echo "[2/5] Resetting any existing nodes (if reachable)..."
for n in "${CP_ARR[@]}"; do talosctl reset --nodes "$n" --reboot || true; done
for n in "${WK_ARR[@]}"; do talosctl reset --nodes "$n" --reboot || true; done

echo "[3/5] Applying control-plane configs..."
for n in "${CP_ARR[@]}"; do talosctl apply-config --insecure --nodes "$n" --file "$OUT_DIR/controlplane.yaml"; done

echo "[4/5] Bootstrapping etcd on first CP node..."
talosctl --nodes "${CP_ARR[0]}" bootstrap

if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  echo "[5/5] Applying worker configs..."
  for n in "${WK_ARR[@]}"; do talosctl apply-config --insecure --nodes "$n" --file "$OUT_DIR/worker.yaml"; done
fi

echo "Fetching kubeconfig..."
talosctl kubeconfig --nodes "${CP_ARR[0]}" --force

echo "Done. Verify with: kubectl get nodes -o wide"
