#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"

NS=${K8S_NAMESPACE:-default}
SECRET=${K8S_IMAGE_PULL_SECRET:-regcreds}

if [ -z "${DOCKER_SERVER:-}" ] || [ -z "${DOCKER_USER:-}" ] || [ -z "${DOCKER_PASS:-}" ]; then
  echo "Set DOCKER_SERVER, DOCKER_USER, DOCKER_PASS in .env (and optional DOCKER_EMAIL)." >&2
  exit 2
fi

kubectl get ns "$NS" >/dev/null 2>&1 || kubectl create ns "$NS"
kubectl -n "$NS" delete secret "$SECRET" >/dev/null 2>&1 || true
kubectl -n "$NS" create secret docker-registry "$SECRET" \
  --docker-server="$DOCKER_SERVER" \
  --docker-username="$DOCKER_USER" \
  --docker-password="$DOCKER_PASS" \
  ${DOCKER_EMAIL:+--docker-email="$DOCKER_EMAIL"}
echo "Created imagePullSecret $SECRET in namespace $NS"
