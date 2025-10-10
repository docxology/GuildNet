#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib-talos.sh"

need kubectl
need talosctl

# Always fetch kubeconfig first and use it explicitly
KCFG="${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}"
mkdir -p "$(dirname "$KCFG")"

cd "$ROOT"
echo "Fetching kubeconfig..."
# This writes a kubeconfig file to the current working directory by default (usually $ROOT)
# so we fetch first, then move it into the user-scoped location.
talosctl kubeconfig --endpoints "$FIRST_HOST:$FIRST_PORT" --nodes "$FIRST_REAL_CP" --force || true

# Migrate repo-local kubeconfig to user-specific path if present
if [ -s "$ROOT/kubeconfig" ]; then
  mv -f "$ROOT/kubeconfig" "$KCFG"
fi

if [ -s "$KCFG" ]; then
  echo "Using kubeconfig: $KCFG"
  echo "Server(s):"; awk '/server: /{print $2}' "$KCFG" || true
  export KUBECONFIG="$KCFG"
else
  echo "WARNING: kubeconfig not yet available; will still attempt readiness checks."
fi

# [7/7] Wait for Kubernetes API using the explicit kubeconfig
wait_kube() {
  local tries=${KUBE_READY_TRIES:-90}; local delay=${KUBE_READY_DELAY:-5}
  while (( tries > 0 )); do
    # Prefer a lightweight raw endpoint first
    if kubectl --request-timeout=5s get --raw='/readyz?verbose' >/dev/null 2>&1; then
      kubectl get nodes -o wide || true
      return 0
    fi
    # Fallback: nodes listing
    if kubectl --request-timeout=5s get nodes >/dev/null 2>&1; then
      kubectl get nodes -o wide || true
      return 0
    fi
    ((tries--)); sleep "$delay"
  done
  echo "WARNING: Kubernetes API not ready after wait" >&2
  return 1
}
wait_kube || true

# If DB setup requested, ensure RethinkDB
if [ "$DB_SETUP" = "1" ]; then
  if [ -x "$ROOT/scripts/rethinkdb-setup.sh" ]; then
    "$ROOT/scripts/rethinkdb-setup.sh" || echo "WARNING: RethinkDB setup reported an issue"
  else
    echo "WARNING: rethinkdb-setup.sh not found/executable; skipping DB setup" >&2
  fi
fi
