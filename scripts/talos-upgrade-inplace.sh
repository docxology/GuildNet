#!/usr/bin/env bash
# In-place Talos OS upgrade (preserve data), rolling through nodes.
# Also supports optional Kubernetes control plane upgrade.
#
# Usage:
#   scripts/talos-upgrade-inplace.sh \
#     --image ghcr.io/siderolabs/installer:vX.Y.Z \
#     --nodes <ip1,ip2,...> [--k8s v1.xx.x]
#
set -euo pipefail

IMAGE=""
NODES=""
K8S_VER=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image) IMAGE="$2"; shift 2 ;;
    --nodes) NODES="$2"; shift 2 ;;
    --k8s) K8S_VER="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$IMAGE" || -z "$NODES" ]]; then
  echo "--image and --nodes are required" >&2
  exit 2
fi

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need talosctl

IFS=',' read -r -a NODE_ARR <<< "$NODES"

for n in "${NODE_ARR[@]}"; do
  echo "Upgrading Talos on $n to $IMAGE..."
  talosctl upgrade --nodes "$n" --image "$IMAGE"
  echo "Waiting for node $n to reboot and become ready..."
  sleep 10
  # Basic wait loop
  for i in {1..60}; do
    if talosctl version --nodes "$n" >/dev/null 2>&1; then
      echo "Node $n back online."; break
    fi
    sleep 5
  done
  if [[ "$K8S_VER" != "" ]]; then
    echo "Upgrading Kubernetes on $n to $K8S_VER..."
    talosctl upgrade-k8s --nodes "$n" --to "$K8S_VER" || true
  fi
  echo "Done: $n"
done

echo "All nodes processed. Validate with: kubectl get nodes; talosctl version --nodes <node>"
