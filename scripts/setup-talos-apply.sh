#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib-talos.sh"

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

# [2/7] Reset nodes (best-effort, skip if config empty or nodes unreachable)
echo "[2/7] Resetting any existing nodes (if reachable)..."

# Check if talosctl config exists and is valid
if ! talosctl config info >/dev/null 2>&1; then
  echo "  Skipping node reset - talosctl config not configured"
  echo "  To configure: run 'make setup-talos-config' or set up Talos nodes first"
else
  for i in "${!CP_ARR[@]}"; do
    fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
    echo "  resetting control-plane (node=$real via $host:$port)"
    if ! check_tcp "$host" "$port" 5; then
      echo "    skipping - node not reachable"
    else
      talosctl reset --endpoints "$host:$port" --nodes "$real" --reboot --graceful=false || echo "    reset failed (continuing)"
    fi
  done

  if [[ ${#WK_ARR[@]} -gt 0 ]]; then
    for i in "${!WK_ARR[@]}"; do
      fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" wk)"
      echo "  resetting worker (node=$real via $host:$port)"
      if ! check_tcp "$host" "$port" 5; then
        echo "    skipping - node not reachable"
      else
        talosctl reset --endpoints "$host:$port" --nodes "$real" --reboot --graceful=false || echo "    reset failed (continuing)"
      fi
    done
  fi
fi

# [3/7] Wait nodes become reachable post-reset
echo "[3/7] Waiting for nodes to become reachable (post-reset) ..."
wait_node() {
  local endpoint=$1; local node=$2; local tries=10; local delay=3  # Reduced wait time
  while (( tries > 0 )); do
    if talosctl version --endpoints "$endpoint" --nodes "$node" >/dev/null 2>&1; then
      echo "    node $node is reachable"; return 0
    fi
    ((tries--)); sleep "$delay"
  done
  echo "    node $node not reachable (skipping)"; return 0  # Don't fail
}

# Check if talosctl config exists and is valid
if ! talosctl config info >/dev/null 2>&1; then
  echo "  Skipping node wait - talosctl config not configured"
else
  for i in "${!CP_ARR[@]}"; do
    fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
    if check_tcp "$host" "$port" 3; then
      wait_node "$host:$port" "$real"
    else
      echo "    control-plane $real not reachable (skipping)"
    fi
  done

  if [[ ${#WK_ARR[@]} -gt 0 ]]; then
    for i in "${!WK_ARR[@]}"; do
      fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" wk)"
      if check_tcp "$host" "$port" 3; then
        wait_node "$host:$port" "$real"
      else
        echo "    worker $real not reachable (skipping)"
      fi
    done
  fi
fi

# [4/7] Apply control-plane configs (skip if config missing or nodes unreachable)
echo "[4/7] Applying control-plane configs..."

if [[ ! -f "$OUT_DIR/controlplane.yaml" ]]; then
  echo "  Skipping config apply - controlplane.yaml not found"
else
  for i in "${!CP_ARR[@]}"; do
    fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
    echo "  apply config to control-plane (node=$real via $host:$port)"
    if ! check_tcp "$host" "$port" 5; then
      echo "    skipping - node not reachable"
    else
      tries=$APPLY_RETRIES
      until talosctl apply-config --insecure --endpoints "$host:$port" --nodes "$real" --file "$OUT_DIR/controlplane.yaml"; do
        ((tries--)) || true
        if (( tries <= 0 )); then echo "    failed to apply config (continuing)"; break; fi
        echo "    retrying apply-config in ${APPLY_RETRY_DELAY}s..."; sleep "$APPLY_RETRY_DELAY"
      done
    fi
  done
fi

# [5/7] Bootstrap etcd (idempotent, skip if nodes unreachable)
echo "[5/7] Bootstrapping etcd on first CP node (idempotent)..."
if check_tcp "$FIRST_HOST" "$FIRST_PORT" 5; then
  if ! talosctl get etcdmember --endpoints "$FIRST_HOST:$FIRST_PORT" --nodes "$FIRST_REAL_CP" >/dev/null 2>&1; then
    talosctl --endpoints "$FIRST_HOST:$FIRST_PORT" --nodes "$FIRST_REAL_CP" bootstrap || echo "Bootstrap attempt failed; continuing"
  else
    echo "  etcd appears bootstrapped already"
  fi
else
  echo "  Skipping etcd bootstrap - first CP node not reachable"
fi

# [6/7] Apply worker configs (if any, skip if config missing or nodes unreachable)
if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  echo "[6/7] Applying worker configs..."

  if [[ ! -f "$OUT_DIR/worker.yaml" ]]; then
    echo "  Skipping worker config apply - worker.yaml not found"
  else
    for i in "${!WK_ARR[@]}"; do
      fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" wk)"
      echo "  apply config to worker (node=$real via $host:$port)"
      if ! check_tcp "$host" "$port" 5; then
        echo "    skipping - node not reachable"
      else
        tries=$APPLY_RETRIES
        until talosctl apply-config --insecure --endpoints "$host:$port" --nodes "$real" --file "$OUT_DIR/worker.yaml"; do
          ((tries--)) || true
          if (( tries <= 0 )); then echo "    failed to apply config (continuing)"; break; fi
          echo "    retrying apply-config in ${APPLY_RETRY_DELAY}s..."; sleep "$APPLY_RETRY_DELAY"
        done
      fi
    done
  fi
else
  echo "[6/7] No worker nodes specified, skipping"
fi
