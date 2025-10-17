#!/usr/bin/env bash
set -euo pipefail

IMAGE=${1:-}

if [ -z "$IMAGE" ]; then
  echo "Usage: $0 <image> [cluster-name]"
  exit 2
fi

if command -v microk8s >/dev/null 2>&1; then
  echo "Loading $IMAGE into microk8s containerd"
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker not found; cannot save image for import into microk8s. Please install docker or provide the image inside microk8s." >&2
    exit 1
  fi

  # Create a temporary file (mktemp will create the file safely)
  TMP=$(mktemp /tmp/operator-image-XXXXXX)
  trap 'rm -f "$TMP"' EXIT

  echo "Saving local image $IMAGE to $TMP"
  if ! docker save "$IMAGE" -o "$TMP"; then
    echo "docker save failed; ensure image exists locally: $IMAGE" >&2
    rm -f "$TMP"
    exit 1
  fi

  echo "Importing saved image into microk8s (this may require sudo)"
  # Retry a few times in case containerd is temporarily busy
  max_attempts=6
  backoff=1
  for attempt in $(seq 1 $max_attempts); do
    echolog() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"; }
    echolog "microk8s ctr import attempt $attempt/$max_attempts"
    if sudo microk8s ctr images import "$TMP"; then
      echolog "operator image loaded into microk8s (attempt $attempt)"
      rm -f "$TMP"
      trap - EXIT
      exit 0
    fi
    echolog "microk8s ctr import failed (attempt $attempt); backing off $backoff s"
    sleep $backoff
    backoff=$((backoff * 2))
  done

  echolog "microk8s ctr import failed after $max_attempts attempts" >&2
  rm -f "$TMP"
  exit 1
fi

exit 0
