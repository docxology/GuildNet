#!/usr/bin/env bash
set -euo pipefail

# smoke-workspace.sh: create a Workspace CR (operator path), wait for it to be reconciled,
# port-forward its Service and optionally cleanup. Meant for developer quick-smoke tests.

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TEMPLATE=${TEMPLATE:-$ROOT/scripts/quick-workspace.yaml.tmpl}

NAME=${1:-quick-code-server-ws}
NAMESPACE=${2:-default}
IMAGE=${3:-codercom/code-server:4.9.0}
PORT=${4:-8080}

TMP=$(mktemp /tmp/quick-ws-XXXXXX.yaml)
trap 'rm -f "$TMP"' EXIT

sed -e "s|{{NAME}}|$NAME|g" \
  -e "s|{{NAMESPACE}}|$NAMESPACE|g" \
  -e "s|{{IMAGE}}|$IMAGE|g" \
  -e "s|{{PORT}}|$PORT|g" \
  "$TEMPLATE" > "$TMP"

echo "Applying Workspace from $TMP"
kubectl apply -f "$TMP"

echo "Waiting for operator to reconcile the deployment (timeout 120s)..."
kubectl -n "$NAMESPACE" rollout status deployment/$NAME --timeout=120s || true

echo "Listing pods (label guildnet.io/workspace=$NAME):"
kubectl -n "$NAMESPACE" get pods -l guildnet.io/workspace="$NAME" -o wide

echo "You can port-forward the Service with the command below and open http://localhost:$PORT"
echo "kubectl -n $NAMESPACE port-forward svc/$NAME $PORT:$PORT"

cat <<'EOF'
Notes:
- This script creates a Workspace CR and relies on the operator to create the Deployment/Service.
- The Workspace CR schema does not accept arbitrary env entries; to set runtime secrets (password) prefer using the operator/hostapp UI or a separate Secret mounted by the operator if supported.
- If the operator fails to reconcile, inspect operator logs and the Workspace resource:
  kubectl -n guildnet-system logs -l app=workspace-operator --tail=200
  kubectl -n $NAMESPACE get workspace $NAME -o yaml
EOF

echo "To cleanup: kubectl -n $NAMESPACE delete workspace $NAME"
