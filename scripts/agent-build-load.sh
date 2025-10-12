#!/usr/bin/env bash
set -euo pipefail

# Build the agent image locally and load it into a kind cluster (if present)
# Usage:
#   scripts/agent-build-load.sh [image-tag]
# Default tag: agent:dev

TAG=${1:-agent:dev}

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
IMG_NAME="${TAG}"

echo "Building image ${IMG_NAME} from images/agent..."
docker buildx build --load -t "${IMG_NAME}" "${ROOT_DIR}/images/agent"

if command -v kind >/dev/null 2>&1; then
  KIND_CLUSTER_NAME=${KIND_CLUSTER:-kind}
  if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
    echo "Loading image into kind cluster '${KIND_CLUSTER_NAME}'..."
    kind load docker-image "${IMG_NAME}" --name "${KIND_CLUSTER_NAME}"
  else
    echo "kind cluster '${KIND_CLUSTER_NAME}' not found; skipping kind load."
  fi
else
  echo "kind not installed; skipping kind load."
fi

if command -v minikube >/dev/null 2>&1; then
  if minikube status >/dev/null 2>&1; then
    echo "Loading image into minikube..."
    minikube image load "${IMG_NAME}"
  else
    echo "minikube not running; skipping minikube load."
  fi
else
  echo "minikube not installed; skipping minikube load."
fi

echo "Done."
