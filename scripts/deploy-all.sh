#!/usr/bin/env bash
set -euo pipefail

# deploy-all.sh
# Idempotent, cross-machine deploy wrapper that reproduces the recommended
# deployment for GuildNet: UI deps, certs, headscale (optional), kubernetes
# addons, operator image build/load, operator deploy, hostapp run or deploy,
# and an end-to-end verify step.

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

USE_KIND=${USE_KIND:-0}
NO_HEADSCALE=${NO_HEADSCALE:-0}
NO_UI=${NO_UI:-0}
NO_HOSTAPP=${NO_HOSTAPP:-0}
TIMEOUT=${TIMEOUT:-120}

echolog(){ echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"; }

echolog "Starting deploy-all: use_kind=$USE_KIND no_headscale=$NO_HEADSCALE no_ui=$NO_UI no_hostapp=$NO_HOSTAPP"

if [ "$NO_UI" -ne 1 ]; then
  echolog "Installing UI deps..."
  if [ -d ui/node_modules ]; then
    echolog "ui/node_modules exists; skipping npm ci"
  else
    (cd ui && npm ci) || echolog "npm ci failed (proceeding)"
  fi
fi

echolog "Regenerating local TLS certs (if needed)"
if [ -f certs/server.crt -a -f certs/server.key ] && [ "${FORCE_REGEN_CERTS:-0}" != "1" ]; then
  echolog "server cert/key already present; skip regen unless FORCE_REGEN_CERTS=1"
else
  ./scripts/generate-server-cert.sh -f || echolog "generate-server-cert failed (proceeding)"
fi

if [ "$NO_HEADSCALE" -ne 1 ]; then
  echolog "Bringing up Headscale and router (best-effort)"
  if docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$$'; then
    echolog "guildnet-headscale container already running; skipping headscale up"
  else
    bash ./scripts/headscale-run.sh up || echolog "headscale up failed (proceeding)"
  fi
  bash ./scripts/detect-lan-and-sync-env.sh || echolog "env sync failed (proceeding)"
  bash ./scripts/headscale-bootstrap.sh || echolog "headscale bootstrap failed (proceeding)"
  bash ./scripts/tailscale-router.sh install || echolog "router install failed (proceeding)"
  bash ./scripts/tailscale-router.sh up || echolog "router up failed (proceeding)"
fi

# Ensure Kubernetes API is reachable; if not, optionally create a kind cluster.
echolog "Checking Kubernetes API availability"
if kubectl version --request-timeout=5s >/dev/null 2>&1; then
  echolog "Kubernetes API reachable"
else
  echolog "Kubernetes API not reachable"
  if [ "$USE_KIND" = "1" ] || [ "${USE_KIND}" = "true" ]; then
    echolog "USE_KIND enabled: creating local kind cluster"
    bash ./scripts/kind-setup.sh || { echolog "kind setup failed"; exit 2; }
  else
    echolog "Kubernetes API not reachable and USE_KIND not set; continuing but many steps will be skipped"
  fi
fi

# Deploy k8s addons (metalLB, CRDs, registry secret, DB)
echolog "Deploying k8s addons and CRDs"
if kubectl get configmap -n metallb-system controller -o yaml >/dev/null 2>&1; then
  echolog "metalLB appears installed; skipping"
else
  bash ./scripts/deploy-metallb.sh || echolog "deploy-metallb failed (proceeding)"
fi

make crd-apply || echolog "crd-apply failed (proceeding)"

bash ./scripts/k8s-setup-registry-secret.sh || echolog "registry secret failed (proceeding)"

if kubectl -n guildnet-system get statefulset rethinkdb >/dev/null 2>&1 || kubectl -n default get statefulset rethinkdb >/dev/null 2>&1; then
  echolog "rethinkdb appears present; skipping setup"
else
  bash ./scripts/rethinkdb-setup.sh || echolog "rethinkdb setup failed (proceeding)"
fi

# Build and load operator image for local clusters (kind/microk8s) when USE_KIND=1
if [ "$USE_KIND" = "1" ] || [ "${USE_KIND}" = "true" ]; then
  echolog "Building and loading operator image into local cluster"
  # If the operator image already exists in the local containerd/kind, skip building
  if docker images | awk '{print $1":"$2}' | grep -q "${OPERATOR_IMAGE:-guildnet/hostapp:local}"; then
    echolog "operator image ${OPERATOR_IMAGE:-guildnet/hostapp:local} already present locally; skipping build"
  else
    make operator-build-load || echolog "operator image build/load failed (proceeding)"
  fi
fi

echolog "Deploying operator into cluster"
if kubectl -n guildnet-system get deployment workspace-operator >/dev/null 2>&1; then
  echolog "workspace-operator deployment exists; attempting rollout restart"
  kubectl -n guildnet-system rollout restart deployment workspace-operator || echolog "operator rollout restart failed"
else
  bash ./scripts/deploy-operator.sh || echolog "deploy-operator failed"
fi

if [ "$NO_HOSTAPP" -ne 1 ]; then
  echolog "Starting hostapp locally (developer flow)"
  bash ./scripts/run-hostapp.sh || echolog "run-hostapp failed (proceeding)"
else
  echolog "Skipping local hostapp run per NO_HOSTAPP"
fi

echolog "Running optional verification (verify-e2e.sh)"
bash ./scripts/verify-e2e.sh || echolog "verify-e2e.sh reported issues"

echolog "deploy-all complete"
