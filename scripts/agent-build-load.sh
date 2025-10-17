#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   scripts/agent-build-load.sh [image-tag]
# Default tag: agent:dev

TAG=${1:-agent:dev}

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
IMG_NAME="${TAG}"

echo "Building image ${IMG_NAME} from images/agent..."
docker buildx build --load -t "${IMG_NAME}" "${ROOT_DIR}/images/agent"

echo "Done."
