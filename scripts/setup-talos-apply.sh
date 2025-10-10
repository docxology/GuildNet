#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib-talos.sh"

need talosctl

# [2/7] Reset nodes (best-effort)
echo "[2/7] Resetting any existing nodes (if reachable)..."
for i in "${!CP_ARR[@]}"; do
  fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
  echo "  resetting control-plane (node=$real via $host:$port)"
  talosctl reset --endpoints "$host:$port" --nodes "$real" --reboot --graceful=false || true
done
if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  for i in "${!WK_ARR[@]}"; do
    fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" wk)"
    echo "  resetting worker (node=$real via $host:$port)"
    talosctl reset --endpoints "$host:$port" --nodes "$real" --reboot --graceful=false || true
  done
fi

# [3/7] Wait nodes become reachable post-reset
echo "[3/7] Waiting for nodes to become reachable (post-reset) ..."
wait_node() {
  local endpoint=$1; local node=$2; local tries=60; local delay=5
  while (( tries > 0 )); do
    if talosctl version --endpoints "$endpoint" --nodes "$node" >/dev/null 2>&1; then
      echo "    node $node is reachable"; return 0
    fi
    ((tries--)); sleep "$delay"
  done
  echo "WARNING: node $node not reachable after wait" >&2; return 1
}
for i in "${!CP_ARR[@]}"; do fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"; wait_node "$host:$port" "$real" || true; done
if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  for i in "${!WK_ARR[@]}"; do fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" wk)"; wait_node "$host:$port" "$real" || true; done
fi

# [4/7] Apply control-plane configs
echo "[4/7] Applying control-plane configs..."
for i in "${!CP_ARR[@]}"; do
  fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
  echo "  apply config to control-plane (node=$real via $host:$port)"
  tries=$APPLY_RETRIES
  until talosctl apply-config --insecure --endpoints "$host:$port" --nodes "$real" --file "$OUT_DIR/controlplane.yaml"; do
    ((tries--)) || true
    if (( tries <= 0 )); then echo "ERROR: failed to apply control-plane config to $real via $host:$port after retries" >&2; exit 1; fi
    echo "  retrying apply-config in ${APPLY_RETRY_DELAY}s..."; sleep "$APPLY_RETRY_DELAY"
  done
done

# [5/7] Bootstrap etcd (idempotent)
echo "[5/7] Bootstrapping etcd on first CP node (idempotent)..."
if ! talosctl get etcdmember --endpoints "$FIRST_HOST:$FIRST_PORT" --nodes "$FIRST_REAL_CP" >/dev/null 2>&1; then
  talosctl --endpoints "$FIRST_HOST:$FIRST_PORT" --nodes "$FIRST_REAL_CP" bootstrap || echo "Bootstrap attempt failed; continuing"
else
  echo "  etcd appears bootstrapped already"
fi

# [6/7] Apply worker configs (if any)
if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  echo "[6/7] Applying worker configs..."
  for i in "${!WK_ARR[@]}"; do
    fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" wk)"
    echo "  apply config to worker (node=$real via $host:$port)"
    tries=$APPLY_RETRIES
    until talosctl apply-config --insecure --endpoints "$host:$port" --nodes "$real" --file "$OUT_DIR/worker.yaml"; do
      ((tries--)) || true
      if (( tries <= 0 )); then echo "ERROR: failed to apply worker config to $real via $host:$port after retries" >&2; exit 1; fi
      echo "  retrying apply-config in ${APPLY_RETRY_DELAY}s..."; sleep "$APPLY_RETRY_DELAY"
    done
  done
else
  echo "[6/7] No worker nodes specified, skipping"
fi
