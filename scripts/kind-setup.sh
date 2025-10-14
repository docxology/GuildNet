#!/usr/bin/env bash
set -euo pipefail
# Create a local kind cluster and ensure kubeconfig is written to ~/.guildnet/kubeconfig
# Defaults:
#   CLUSTER_NAME=guildnet
#   POD_SUBNET=10.244.0.0/16
#   SVC_SUBNET=10.96.0.0/12
#   KUBECONFIG_OUT=$HOME/.guildnet/kubeconfig

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"

CLUSTER_NAME=${KIND_CLUSTER_NAME:-${CLUSTER_NAME:-guildnet}}
POD_SUBNET=${KIND_POD_SUBNET:-10.244.0.0/16}
SVC_SUBNET=${KIND_SVC_SUBNET:-10.96.0.0/12}
# Allow overriding the API server host port (useful when 6443 is already bound on local host)
KIND_API_SERVER_PORT=${KIND_API_SERVER_PORT:-6443}
KUBECONFIG_OUT=${KUBECONFIG_OUT:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need docker
need bash
need curl

# Use local kind if available; otherwise, install a pinned kind binary into ~/.guildnet/bin
run_kind() {
  if command -v kind >/dev/null 2>&1; then
    kind "$@"
    return
  fi
  KIND_BIN="$HOME/.guildnet/bin/kind"
  if [ ! -x "$KIND_BIN" ]; then
    echo "[kind-setup] Installing kind binary (linux-amd64) into $HOME/.guildnet/bin ..."
    mkdir -p "$HOME/.guildnet/bin"
    curl -fsSL -o "$KIND_BIN" "https://github.com/kubernetes-sigs/kind/releases/download/v0.25.0/kind-linux-amd64"
    chmod +x "$KIND_BIN"
  fi
  "$KIND_BIN" "$@"
}

# Create cluster config
tmpcfg=$(mktemp)
cat >"$tmpcfg" <<YAML
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  podSubnet: ${POD_SUBNET}
  serviceSubnet: ${SVC_SUBNET}
  apiServerAddress: 127.0.0.1
  apiServerPort: ${KIND_API_SERVER_PORT}
nodes:
  - role: control-plane
  - role: worker
  - role: worker
YAML

# Create cluster if missing
if ! run_kind get clusters | grep -qx "$CLUSTER_NAME"; then
  echo "Creating kind cluster '$CLUSTER_NAME'..."
  run_kind create cluster --name "$CLUSTER_NAME" --config "$tmpcfg"
else
  echo "kind cluster '$CLUSTER_NAME' already exists; ensuring kubeconfig"
fi
rm -f "$tmpcfg"

mkdir -p "$(dirname "$KUBECONFIG_OUT")"
run_kind get kubeconfig --name "$CLUSTER_NAME" >"$KUBECONFIG_OUT"
chmod 600 "$KUBECONFIG_OUT"
export KUBECONFIG="$KUBECONFIG_OUT"

# Wait for readiness
kubectl --request-timeout=5s cluster-info >/dev/null
kubectl wait --for=condition=Ready nodes --all --timeout=180s || true
kubectl get nodes -o wide

echo "kind cluster ready; kubeconfig: $KUBECONFIG_OUT"
