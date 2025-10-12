#!/usr/bin/env bash
set -euo pipefail

# Setup or verify RethinkDB exposure in the cluster.
# - Applies k8s/rethinkdb.yaml
# - Waits briefly for LoadBalancer IP via MetalLB
# - If no LB ingress appears, optionally patches Service to NodePort and prints reachable address
#
# Env knobs:
#   NAMESPACE                 Namespace to use (default: default)
#   SERVICE_NAME              Service name (default: rethinkdb)
#   WAIT_SECONDS              Seconds to wait for LB ingress (default: 30)
#   NODEPORT_FALLBACK         When no LB is assigned, fallback to NodePort (default: true)
#   NODE_SELECTOR_LABEL       Optional label selector to prefer a specific node for NodePort (e.g., "kubernetes.io/hostname=my-node")

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"

# Ensure kubectl uses the GuildNet kubeconfig when available
export KUBECONFIG="${KUBECONFIG:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}"

NAMESPACE=${RETHINKDB_NAMESPACE:-${NAMESPACE:-default}}
SERVICE_NAME=${RETHINKDB_SERVICE_NAME:-${SERVICE_NAME:-rethinkdb}}
WAIT_SECONDS=${WAIT_SECONDS:-30}
NODEPORT_FALLBACK=${NODEPORT_FALLBACK:-true}

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required" >&2
  exit 1
fi

set -x
kubectl apply -n "$NAMESPACE" -f "$REPO_ROOT/k8s/rethinkdb.yaml"
set +x

echo "Waiting up to ${WAIT_SECONDS}s for LoadBalancer IP..."
end=$((SECONDS + WAIT_SECONDS))
LB_HOST=""
while [ $SECONDS -lt $end ]; do
  ip=$(kubectl get svc "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
  host=$(kubectl get svc "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)
  if [ -n "$ip" ] || [ -n "$host" ]; then
    LB_HOST=${ip:-$host}
    break
  fi
  sleep 2
done

CLIENT_PORT=$(kubectl get svc "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{range .spec.ports[*]}{.port}{" "}{.name}{"\n"}{end}' | awk '$2=="client"{print $1; found=1} END{if(!found) print "28015"}')

if [ -n "$LB_HOST" ]; then
  echo "RethinkDB reachable at: ${LB_HOST}:${CLIENT_PORT}"
  echo "export RETHINKDB_ADDR=${LB_HOST}:${CLIENT_PORT}"
  exit 0
fi

echo "No LoadBalancer ingress assigned."
if [ "${NODEPORT_FALLBACK}" = "true" ] || [ "${NODEPORT_FALLBACK}" = "1" ]; then
  echo "Patching Service to NodePort for external access..."
  kubectl patch svc "$SERVICE_NAME" -n "$NAMESPACE" -p '{"spec":{"type":"NodePort"}}' >/dev/null
  NODE_PORT=$(kubectl get svc "$SERVICE_NAME" -n "$NAMESPACE" -o jsonpath='{range .spec.ports[*]}{.nodePort}{" "}{.name}{"\n"}{end}' | awk '$2=="client"{print $1; found=1} END{if(!found) print $1}')
  # Choose a node IP: prefer ExternalIP then InternalIP
  SEL=""
  if [ -n "${NODE_SELECTOR_LABEL:-}" ]; then
    SEL="-l ${NODE_SELECTOR_LABEL}"
  fi
  NODE_IP=$(kubectl get nodes $SEL -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="ExternalIP")].address}{"\n"}{end}' | awk 'NF>0{print; exit}')
  if [ -z "$NODE_IP" ]; then
    NODE_IP=$(kubectl get nodes $SEL -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="InternalIP")].address}{"\n"}{end}' | awk 'NF>0{print; exit}')
  fi
  if [ -z "$NODE_IP" ] || [ -z "$NODE_PORT" ]; then
    echo "Failed to determine NodePort address" >&2
    exit 2
  fi
  echo "RethinkDB reachable at: ${NODE_IP}:${NODE_PORT}"
  echo "export RETHINKDB_ADDR=${NODE_IP}:${NODE_PORT}"
  exit 0
else
  echo "NODEPORT_FALLBACK disabled and no LB available; cannot provide external address." >&2
  exit 3
fi
