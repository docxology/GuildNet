#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib-talos.sh"

need kubectl
# Install talosctl if missing
if ! command -v talosctl >/dev/null 2>&1; then
  echo "talosctl not found, installing..."
  case "$(uname -s)" in
    Linux)
      os="linux" arch=""
      case "$(uname -m)" in
        x86_64|amd64) arch=amd64;;
        aarch64|arm64) arch=arm64;;
        *) arch=amd64;;
      esac
      url="https://github.com/siderolabs/talos/releases/latest/download/talosctl-${os}-${arch}"
      echo "Downloading talosctl from $url"
      curl -fsSL -o /tmp/talosctl "$url"
      chmod +x /tmp/talosctl
      sudo install -m 0755 /tmp/talosctl /usr/local/bin/talosctl
      ;;
    Darwin)
      if command -v brew >/dev/null 2>&1; then
        brew install siderolabs/tap/talosctl
      else
        echo "Please install Homebrew from https://brew.sh and run: brew install siderolabs/tap/talosctl"
        exit 1
      fi
      ;;
    *)
      echo "Please install talosctl for your platform from https://www.talos.dev/latest/introduction/quickstart/"
      exit 1
      ;;
  esac
  echo "talosctl installed successfully"
fi

# Always fetch kubeconfig first and use it explicitly (skip if nodes unreachable)
KCFG="${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}"
mkdir -p "$(dirname "$KCFG")"

cd "$ROOT"
echo "Fetching kubeconfig..."

# Only fetch kubeconfig if first CP node is reachable
if check_tcp "$FIRST_HOST" "$FIRST_PORT" 5; then
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
else
  echo "  Skipping kubeconfig fetch - first CP node not reachable"
  echo "  To get kubeconfig later: run 'make setup-talos-wait-kube' when nodes are ready"
fi

# [7/7] Wait for Kubernetes API using the explicit kubeconfig (skip if no kubeconfig)
wait_kube() {
  # Only wait if we have a kubeconfig
  if [[ -z "${KUBECONFIG:-}" ]] || [[ ! -f "${KUBECONFIG:-}" ]]; then
    echo "  Skipping Kubernetes readiness check - no kubeconfig available"
    echo "  To check later: run 'make setup-talos-wait-kube' when nodes are ready"
    return 0
  fi

  local tries=${KUBE_READY_TRIES:-10}; local delay=${KUBE_READY_DELAY:-3}  # Reduced wait time
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
  echo "  Kubernetes API not ready after wait (skipping)"
  return 0  # Don't fail the setup
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
