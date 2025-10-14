#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"
OP_IMAGE=${OPERATOR_IMAGE:-ghcr.io/your/module/hostapp:latest}
CLUSTER_NAME=${KIND_CLUSTER_NAME:-${CLUSTER_NAME:-guildnet}}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker required"; exit 2
fi

echo "Building operator image: $OP_IMAGE"
docker build -f "$ROOT/scripts/Dockerfile.operator" -t "$OP_IMAGE" "$ROOT"

if command -v kind >/dev/null 2>&1; then
  if kind get clusters | grep -qx "$CLUSTER_NAME"; then
    echo "Loading $OP_IMAGE into kind cluster $CLUSTER_NAME"
    kind load docker-image "$OP_IMAGE" --name "$CLUSTER_NAME"
  else
    echo "Kind cluster $CLUSTER_NAME not found; skipping kind load"
  fi
else
  echo "kind not installed; skipping image load"
fi

echo "Done"
