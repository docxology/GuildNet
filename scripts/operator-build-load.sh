#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"
OP_IMAGE=${OPERATOR_IMAGE:-ghcr.io/your/module/hostapp:latest}
CLUSTER_NAME=${CLUSTER_NAME:-guildnet}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker required"; exit 2
fi

echo "Building operator image: $OP_IMAGE"
docker build -f "$ROOT/scripts/Dockerfile.operator" -t "$OP_IMAGE" "$ROOT"

if command -v microk8s >/dev/null 2>&1; then
  echo "Importing $OP_IMAGE into microk8s"
  # Delegate to the same load helper used elsewhere which handles microk8s import
  bash "$ROOT/scripts/load-operator-image.sh" "$OP_IMAGE" "" || echo "microk8s import helper failed"
else
  echo "No supported local cluster loader found; image build complete."
fi

echo "Done"
