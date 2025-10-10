#!/usr/bin/env bash
# Fresh Talos cluster deployment helper (wipe & recreate).
# Assumes talosctl is installed and accessible.
#
# Usage:
#   scripts/talos-fresh-deploy.sh \
#     --cluster mycluster \
#     --endpoint https://<control-plane-endpoint>:6443 \
#     --cp <cp1-ip,cp2-ip,...> \
#     --workers <w1-ip,w2-ip,...>
#
set -euo pipefail

# Load repo-level environment defaults if present
REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -f "$REPO_ROOT/.env" ]; then
  # shellcheck disable=SC1090
  . "$REPO_ROOT/.env"
fi

# Tunable defaults (override in .env)
PRECHECK_PORT=${PRECHECK_PORT:-50000}
PRECHECK_TIMEOUT=${PRECHECK_TIMEOUT:-3}          # seconds per TCP check
PRECHECK_MAX_WAIT_SECS=${PRECHECK_MAX_WAIT_SECS:-600}
PRECHECK_PING=${PRECHECK_PING:-0}               # 1 to attempt ICMP ping
APPLY_RETRIES=${APPLY_RETRIES:-10}
APPLY_RETRY_DELAY=${APPLY_RETRY_DELAY:-5}
KUBE_READY_TRIES=${KUBE_READY_TRIES:-90}
KUBE_READY_DELAY=${KUBE_READY_DELAY:-5}
REQUIRE_ENDPOINT_MATCH_CP=${REQUIRE_ENDPOINT_MATCH_CP:-0}
DB_SETUP=${DB_SETUP:-1}                          # 1 to run rethinkdb-setup.sh at the end

CLUSTER="${CLUSTER:-mycluster}"
ENDPOINT="${ENDPOINT:-}"
CP_NODES="${CP_NODES:-}"
WK_NODES="${WK_NODES:-}"
# Optional: real node IPs (without ports) to use for talosctl --nodes when CP/WK_NODES are host:port forwards
CP_NODES_REAL="${CP_NODES_REAL:-}"
WK_NODES_REAL="${WK_NODES_REAL:-}"
OUT_DIR="./talos"
FORCE=0
declare -a WK_ARR=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster) CLUSTER="$2"; shift 2 ;;
    --endpoint) ENDPOINT="$2"; shift 2 ;;
    --cp) CP_NODES="$2"; shift 2 ;;
    --workers) WK_NODES="$2"; shift 2 ;;
    --out) OUT_DIR="$2"; shift 2 ;;
    --force) FORCE=1; shift 1 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

# Allow alternate env var names if provided (back-compat / clarity)
if [[ -z "$ENDPOINT" && -n "${TALOS_ENDPOINT:-}" ]]; then ENDPOINT="$TALOS_ENDPOINT"; fi
if [[ -z "$CP_NODES" && -n "${TALOS_CP_NODES:-}" ]]; then CP_NODES="$TALOS_CP_NODES"; fi
if [[ -z "$WK_NODES" && -n "${TALOS_WORKER_NODES:-}" ]]; then WK_NODES="$TALOS_WORKER_NODES"; fi

# Provide opinionated defaults if values are still empty (developer-friendly)
if [[ -z "$ENDPOINT" ]]; then ENDPOINT="https://10.0.0.10:6443"; fi
if [[ -z "$CP_NODES" ]]; then CP_NODES="10.0.0.10"; fi
if [[ -z "$WK_NODES" ]]; then WK_NODES="10.0.0.20"; fi
echo "Using cluster settings: CLUSTER=$CLUSTER ENDPOINT=$ENDPOINT CP_NODES=$CP_NODES WK_NODES=$WK_NODES"

# Validate ENDPOINT format and consistency (best-effort)
if ! printf '%s' "$ENDPOINT" | grep -Eq '^https://'; then
  echo "WARN: ENDPOINT does not start with https:// (value: $ENDPOINT)" >&2
fi
endpoint_host=$(printf '%s' "$ENDPOINT" | sed -E 's#^https?://([^:/]+).*#\1#')
IFS=',' read -r -a _cp_check <<< "$CP_NODES"
first_cp="${_cp_check[0]:-}"
if [ -n "$first_cp" ] && [ -n "$endpoint_host" ] && [ "$REQUIRE_ENDPOINT_MATCH_CP" = "1" ] && [ "$endpoint_host" != "$first_cp" ]; then
  echo "ERROR: ENDPOINT host ($endpoint_host) does not match first control-plane IP ($first_cp) and REQUIRE_ENDPOINT_MATCH_CP=1" >&2
  exit 2
fi

if [[ -z "$ENDPOINT" || -z "$CP_NODES" ]]; then
  echo "--endpoint and --cp are required" >&2
  exit 2
fi

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need talosctl
need kubectl

# helpers
parse_host_port() { # input -> host port
  local in="$1"
  if [[ "$in" == *:* ]]; then
    printf '%s %s' "${in%%:*}" "${in##*:}"
  else
    printf '%s %s' "$in" "$PRECHECK_PORT"
  fi
}

real_ip_for() { # forward_host forward_port idx kind
  local fhost="$1" fport="$2" idx="$3" kind="$4" # kind=cp|wk
  local -a real_list=()
  local IFS=','
  if [[ "$kind" == "cp" && -n "$CP_NODES_REAL" ]]; then real_list=( $CP_NODES_REAL ); fi
  if [[ "$kind" == "wk" && -n "$WK_NODES_REAL" ]]; then real_list=( $WK_NODES_REAL ); fi
  if [[ ${#real_list[@]} -gt 0 ]]; then
    echo "${real_list[$idx]}"; return 0
  fi
  # heuristic fallback: map common forwarded ports to 10.0.0.x
  case "$fport" in
    50010) echo "10.0.0.10"; return 0 ;;
    50020) echo "10.0.0.20"; return 0 ;;
  esac
  # last resort: if input looked like host:port, strip port
  echo "$fhost"
}

# Preflight: basic TCP reachability test for Talos API on nodes
check_tcp() { # host port timeout_seconds
  local h="$1" p="$2" t="${3:-3}"
  if command -v nc >/dev/null 2>&1; then
    nc -z -G "$t" "$h" "$p" >/dev/null 2>&1
  else
    # bash /dev/tcp trick as fallback if available
    (exec 3<>"/dev/tcp/$h/$p") >/dev/null 2>&1 || return 1
  fi
}

# If nodes appear unreachable, fail fast with guidance and optional local fallback when AUTO_LOCAL=1
echo "[0/7] Preflight: checking node reachability on TCP :$PRECHECK_PORT"
UNREACH=0

if [[ -n "$CP_NODES" ]]; then
  IFS=',' read -r -a CP_ARR <<< "$CP_NODES"
  for n in "${CP_ARR[@]}"; do
    read -r host port <<< "$(parse_host_port "$n")"
    if ! check_tcp "$host" "$port" "$PRECHECK_TIMEOUT"; then
      echo "  WARN: control-plane $host not reachable on :$port"
      UNREACH=1
    else
      echo "  OK: control-plane $host:$port reachable"
    fi
    if [ "$PRECHECK_PING" = "1" ] && command -v ping >/dev/null 2>&1; then
      if ping -c 1 -W 2 "$host" >/dev/null 2>&1; then echo "    ping ok"; else echo "    ping failed"; fi
    fi
  done
fi
if [[ -n "$WK_NODES" ]]; then
  IFS=',' read -r -a WK_ARR <<< "$WK_NODES"
  for n in "${WK_ARR[@]}"; do
    read -r host port <<< "$(parse_host_port "$n")"
    if ! check_tcp "$host" "$port" "$PRECHECK_TIMEOUT"; then
      echo "  WARN: worker $host not reachable on :$port"
      UNREACH=1
    else
      echo "  OK: worker $host:$port reachable"
    fi
    if [ "$PRECHECK_PING" = "1" ] && command -v ping >/dev/null 2>&1; then
      if ping -c 1 -W 2 "$host" >/dev/null 2>&1; then echo "    ping ok"; else echo "    ping failed"; fi
    fi
  done
fi
if [[ $UNREACH -eq 1 ]]; then
  echo "WARN: One or more nodes are unreachable on TCP :$PRECHECK_PORT. Will wait up to ${PRECHECK_MAX_WAIT_SECS}s for nodes to come online..."
  deadline=$((SECONDS + PRECHECK_MAX_WAIT_SECS))
  while (( SECONDS < deadline )); do
    all_ok=1
    if [[ -n "$CP_NODES" ]]; then
      IFS=',' read -r -a CP_ARR <<< "$CP_NODES"
      for n in "${CP_ARR[@]}"; do
        read -r host port <<< "$(parse_host_port "$n")"
        if ! check_tcp "$host" "$port" "$PRECHECK_TIMEOUT"; then all_ok=0; break; fi
      done
    fi
    if [[ $all_ok -eq 1 && -n "$WK_NODES" ]]; then
      IFS=',' read -r -a WK_ARR <<< "$WK_NODES"
      for n in "${WK_ARR[@]}"; do
        read -r host port <<< "$(parse_host_port "$n")"
        if ! check_tcp "$host" "$port" "$PRECHECK_TIMEOUT"; then all_ok=0; break; fi
      done
    fi
    if [[ $all_ok -eq 1 ]]; then break; fi
    sleep 5
  done
  if [[ $all_ok -ne 1 ]]; then
    echo "ERROR: Nodes still unreachable after wait. Ensure Talos live OS is booted and network allows TCP :$PRECHECK_PORT." >&2
    exit 1
  fi
fi

IFS=',' read -r -a CP_ARR <<< "$CP_NODES"
# Allow empty workers gracefully; if WK_NODES is empty or unset after read, define empty array
if [[ -n "${WK_NODES}" ]]; then
  IFS=',' read -r -a WK_ARR <<< "$WK_NODES"
else
  WK_ARR=()
fi

# End-to-end overlay check: verify Talos maintenance API is reachable through the forwarder
echo "[1.5/7] Verifying Talos maintenance API via forwarder (talosctl version)..."
overlay_ok=1
for i in "${!CP_ARR[@]}"; do
  fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
  echo "  checking control-plane node=$real via $host:$port"
  if ! talosctl version -e "$host:$port" -n "$real" -i --short >/dev/null 2>&1; then
    echo "    ERROR: talosctl couldn't reach $real via $host:$port (overlay route likely missing)"
    overlay_ok=0
  else
    echo "    OK: talosctl maintenance API reachable"
  fi
done
if [[ $overlay_ok -ne 1 ]]; then
  cat >&2 <<'MSG'
FATAL: Unable to reach Talos maintenance API through the tsnet forwarder.

What this means:
- Your local forwarder is listening on localhost, but it can't dial 10.0.0.0/24 over the tailnet.
- You need a subnet router inside the 10.0.0.0/24 network (or on a host with L2/L3 reachability to the nodes)
  that advertises the route 10.0.0.0/24 to your Headscale/Tailscale control plane.

How to fix:
1) On any machine that can reach 10.0.0.10/20 directly, run a Tailscale/Headscale client and advertise routes:
   tailscale up --login-server=$TS_LOGIN_SERVER --authkey=... --advertise-routes=10.0.0.0/24 --accept-routes
   -or- build & run our helper: make tsnet-subnet-router && make run-subnet-router (must run on that LAN host)
2) Re-run this script. The [1.5/7] check should pass before proceeding.
MSG
  exit 1
fi

mkdir -p "$OUT_DIR"

echo "[1/7] Generating cluster config..."
if [[ $FORCE -eq 1 ]]; then
  echo "  --force specified: regenerating config into $OUT_DIR" >&2
  talosctl gen config "$CLUSTER" "$ENDPOINT" --output-dir "$OUT_DIR" --force
else
  if [[ -f "$OUT_DIR/controlplane.yaml" ]]; then
    echo "  existing config detected (use --force to regenerate); skipping generation"
  else
    talosctl gen config "$CLUSTER" "$ENDPOINT" --output-dir "$OUT_DIR"
  fi
fi

echo "[2/7] Resetting any existing nodes (if reachable)..."
# reset per-node using forwarded endpoint and real node IP
for i in "${!CP_ARR[@]}"; do
  fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"
  real="$(real_ip_for "$host" "$port" "$i" cp)"
  echo "  resetting control-plane (node=$real via $host:$port)"
  talosctl reset --endpoints "$host:$port" --nodes "$real" --reboot --graceful=false || true
done
if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  for i in "${!WK_ARR[@]}"; do
    fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"
    real="$(real_ip_for "$host" "$port" "$i" wk)"
    echo "  resetting worker (node=$real via $host:$port)"
    talosctl reset --endpoints "$host:$port" --nodes "$real" --reboot --graceful=false || true
  done
fi

echo "[3/7] Waiting for nodes to become reachable (post-reset) ..."
wait_node() {
  local endpoint=$1; local node=$2; local tries=60; local delay=5
  while (( tries > 0 )); do
    if talosctl version --endpoints "$endpoint" --nodes "$node" >/dev/null 2>&1; then
      echo "    node $node is reachable"
      return 0
    fi
    ((tries--))
    sleep "$delay"
  done
  echo "WARNING: node $node not reachable after wait" >&2
  return 1
}
for i in "${!CP_ARR[@]}"; do fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"; wait_node "$host:$port" "$real" || true; done
if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  for i in "${!WK_ARR[@]}"; do fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" wk)"; wait_node "$host:$port" "$real" || true; done
fi

echo "[4/7] Applying control-plane configs..."
for i in "${!CP_ARR[@]}"; do
  fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
  echo "  apply config to control-plane (node=$real via $host:$port)"
  tries=$APPLY_RETRIES
  until talosctl apply-config --insecure --endpoints "$host:$port" --nodes "$real" --file "$OUT_DIR/controlplane.yaml"; do
    ((tries--)) || true
    if (( tries <= 0 )); then
      echo "ERROR: failed to apply control-plane config to $real via $host:$port after retries" >&2
      exit 1
    fi
    echo "  retrying apply-config for $n in ${APPLY_RETRY_DELAY}s..."; sleep "$APPLY_RETRY_DELAY"
  done
done

echo "[5/7] Bootstrapping etcd on first CP node (idempotent)..."
# Use forwarded endpoint and real node IP for bootstrap/get checks
first_cp_fwd="${CP_ARR[0]}"
read -r first_host first_port <<< "$(parse_host_port "$first_cp_fwd")"
first_real_cp="$(real_ip_for "$first_host" "$first_port" 0 cp)"
if ! talosctl get etcdmember --endpoints "$first_host:$first_port" --nodes "$first_real_cp" >/dev/null 2>&1; then
  talosctl --endpoints "$first_host:$first_port" --nodes "$first_real_cp" bootstrap || {
    echo "Bootstrap attempt failed; will still proceed (may already be bootstrapped)" >&2
  }
else
  echo "  etcd appears bootstrapped already"
fi

if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  echo "[6/7] Applying worker configs..."
  for i in "${!WK_ARR[@]}"; do
    fwd="${WK_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" wk)"
    echo "  apply config to worker (node=$real via $host:$port)"
    tries=$APPLY_RETRIES
    until talosctl apply-config --insecure --endpoints "$host:$port" --nodes "$real" --file "$OUT_DIR/worker.yaml"; do
      ((tries--)) || true
      if (( tries <= 0 )); then
        echo "ERROR: failed to apply worker config to $real via $host:$port after retries" >&2
        exit 1
      fi
      echo "  retrying apply-config for $n in ${APPLY_RETRY_DELAY}s..."; sleep "$APPLY_RETRY_DELAY"
    done
  done
else
  echo "[6/7] No worker nodes specified, skipping"
fi

echo "[7/7] Waiting for Kubernetes API (kubelet nodes Ready)..."
wait_kube() {
  local tries=90; local delay=5
  while (( tries > 0 )); do
    if kubectl get nodes >/dev/null 2>&1; then
      kubectl get nodes -o wide || true
      return 0
    fi
    ((tries--))
    sleep "$delay"
  done
  echo "WARNING: Kubernetes API not ready after wait" >&2
  return 1
}
wait_kube || true

echo "Fetching kubeconfig..."
# Fetch kubeconfig using the same endpoint/node mapping as above
talosctl kubeconfig --endpoints "$first_host:$first_port" --nodes "$first_real_cp" --force

if [ "$DB_SETUP" = "1" ]; then
  echo "[8/8] Ensuring RethinkDB Service is deployed and reachable..."
  REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
  if [ -x "$REPO_ROOT/scripts/rethinkdb-setup.sh" ]; then
    bash "$REPO_ROOT/scripts/rethinkdb-setup.sh" || {
      echo "WARNING: RethinkDB setup script reported an issue; continuing cluster deploy." >&2
    }
  else
    echo "WARNING: rethinkdb-setup.sh not found/executable; skipping DB setup" >&2
  fi
fi

echo "Done. Verify with: kubectl get nodes -o wide; kubectl get svc rethinkdb -o wide"
