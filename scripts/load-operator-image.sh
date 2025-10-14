#!/usr/bin/env bash
set -euo pipefail

IMAGE=${1:-}
KIND_CLUSTER_NAME=${2:-}

if [ -z "$IMAGE" ]; then
  echo "Usage: $0 <image> [kind-cluster-name]"
  exit 2
fi

# If a kind cluster name is provided, prefer loading into that kind cluster.
if [ -n "$KIND_CLUSTER_NAME" ] && command -v kind >/dev/null 2>&1; then
  echo "Loading $IMAGE into kind cluster $KIND_CLUSTER_NAME"
  kind load docker-image "$IMAGE" --name "$KIND_CLUSTER_NAME"
  echo "Loaded $IMAGE into kind cluster $KIND_CLUSTER_NAME"
  exit 0
fi

# If microk8s is present, prefer loading into microk8s when no kind cluster name was given.
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
  for attempt in 1 2 3; do
    if sudo microk8s ctr images import "$TMP"; then
      echo "operator image loaded into microk8s (attempt $attempt)"
      rm -f "$TMP"
      trap - EXIT
      exit 0
    else
      echo "microk8s ctr import failed (attempt $attempt); retrying..."
      sleep 1
    fi
  done

  echo "microk8s ctr import failed after retries" >&2
  rm -f "$TMP"
  exit 1
fi

# If we reached here and kind exists but no KIND_CLUSTER_NAME was provided, skip kind load
if command -v kind >/dev/null 2>&1; then
  if [ -z "$KIND_CLUSTER_NAME" ]; then
    echo "KIND_CLUSTER_NAME not set; skipping kind load"
    exit 0
  fi
fi

echo "No supported local cluster loader found (kind or microk8s); skipping image load"
exit 0
