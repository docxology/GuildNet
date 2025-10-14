#!/usr/bin/env bash
# expose-workspace.sh <workspace-name> [--type nodeport|loadbalancer]
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"
export KUBECONFIG="${KUBECONFIG:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}"

if [ $# -lt 1 ]; then
  echo "Usage: $0 <workspace-name> [--type nodeport|loadbalancer]"; exit 2
fi
NAME=$1
TYPE_ARG=${2:-}
if [ "${TYPE_ARG}" = "--type" ]; then TYPE_ARG=${3:-}; fi
TYPE=$(echo "${TYPE_ARG:-nodeport}" | tr '[:upper:]' '[:lower:]')
if [ "$TYPE" != "nodeport" ] && [ "$TYPE" != "loadbalancer" ]; then
  echo "Unknown type: $TYPE"; exit 2
fi

NS=${WORKSPACE_NAMESPACE:-default}
# Verify workspace exists
if ! kubectl get workspace "$NAME" -n "$NS" >/dev/null 2>&1; then
  echo "Workspace $NAME not found in namespace $NS"; exit 3
fi

SVC_NAME=$NAME
# If the service exists and already has the desired type, exit
EXISTING_TYPE=$(kubectl get svc "$SVC_NAME" -n "$NS" -o jsonpath='{.spec.type}' 2>/dev/null || echo "")
if [ -n "$EXISTING_TYPE" ] && [ "$(echo $EXISTING_TYPE | tr '[:upper:]' '[:lower:]')" = "$TYPE" ]; then
  echo "Service $SVC_NAME already of type $EXISTING_TYPE in namespace $NS"; exit 0
fi

# Patch service type (note: changing ClusterIP -> NodePort or LoadBalancer is supported)
case "$TYPE" in
  nodeport)
    kubectl patch svc "$SVC_NAME" -n "$NS" -p '{"spec":{"type":"NodePort"}}'
    ;;
  loadbalancer)
    kubectl patch svc "$SVC_NAME" -n "$NS" -p '{"spec":{"type":"LoadBalancer"}}'
    ;;
esac

echo "Patched service $SVC_NAME to type $TYPE in namespace $NS"
kubectl get svc "$SVC_NAME" -n "$NS" -o wide
