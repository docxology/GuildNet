#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib-talos.sh"

need kubectl
need talosctl

# [7/7] Wait for Kubernetes API
wait_kube() {
  local tries=${KUBE_READY_TRIES:-90}; local delay=${KUBE_READY_DELAY:-5}
  while (( tries > 0 )); do
    if kubectl get nodes >/dev/null 2>&1; then
      kubectl get nodes -o wide || true
      return 0
    fi
    ((tries--)); sleep "$delay"
  done
  echo "WARNING: Kubernetes API not ready after wait" >&2
  return 1
}
wait_kube || true

echo "Fetching kubeconfig..."
talosctl kubeconfig --endpoints "$FIRST_HOST:$FIRST_PORT" --nodes "$FIRST_REAL_CP" --force

if [ "$DB_SETUP" = "1" ]; then
  if [ -x "$ROOT/scripts/rethinkdb-setup.sh" ]; then
    "$ROOT/scripts/rethinkdb-setup.sh" || echo "WARNING: RethinkDB setup reported an issue"
  else
    echo "WARNING: rethinkdb-setup.sh not found/executable; skipping DB setup" >&2
  fi
fi
