#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/lib-talos.sh"

need talosctl

# Router health gate (best-effort)
if command -v tailscale >/dev/null 2>&1; then
  if ! tailscale status >/dev/null 2>&1; then
    echo "WARN: tailscale not connected; run: make setup-tailscale" >&2
  fi
fi

echo "[0/7] Preflight: checking node reachability on TCP :$PRECHECK_PORT"
UNREACH=0
if [[ -n "$CP_NODES" ]]; then
  for n in "${CP_ARR[@]}"; do
    read -r host port <<< "$(parse_host_port "$n")"
    if ! check_tcp "$host" "$port" "$PRECHECK_TIMEOUT"; then
      echo "  WARN: control-plane $host not reachable on :$port"; UNREACH=1
    else
      echo "  OK: control-plane $host:$port reachable"
    fi
  done
fi
if [[ ${#WK_ARR[@]} -gt 0 ]]; then
  for n in "${WK_ARR[@]}"; do
    read -r host port <<< "$(parse_host_port "$n")"
    if ! check_tcp "$host" "$port" "$PRECHECK_TIMEOUT"; then
      echo "  WARN: worker $host not reachable on :$port"; UNREACH=1
    else
      echo "  OK: worker $host:$port reachable"
    fi
  done
fi
if [[ $UNREACH -eq 1 ]]; then
  echo "WARN: Nodes unreachable; waiting up to ${PRECHECK_MAX_WAIT_SECS}s ..."
  deadline=$((SECONDS + PRECHECK_MAX_WAIT_SECS))
  while (( SECONDS < deadline )); do
    all_ok=1
    for n in "${CP_ARR[@]}"; do read -r h p <<< "$(parse_host_port "$n")"; check_tcp "$h" "$p" "$PRECHECK_TIMEOUT" || all_ok=0; done
    if [[ $all_ok -eq 1 && ${#WK_ARR[@]} -gt 0 ]]; then
      for n in "${WK_ARR[@]}"; do read -r h p <<< "$(parse_host_port "$n")"; check_tcp "$h" "$p" "$PRECHECK_TIMEOUT" || all_ok=0; done
    fi
    [[ $all_ok -eq 1 ]] && break
    sleep 5
  done
  [[ $all_ok -ne 1 ]] && {
    echo "ERROR: Nodes still unreachable after wait." >&2
    echo "Hints:" >&2
    echo "  - Ensure Talos nodes are booted and reachable (10.0.0.10/20)." >&2
    echo "  - Or use forwarded endpoints in .env (e.g., CP_NODES=127.0.0.1:50010, WK_NODES=127.0.0.1:50020 with *_REAL set)." >&2
    echo "  - Verify router Connected and routes Enabled (make diag-router)." >&2
    exit 1
  }
fi

echo "[1.5/7] Verifying Talos maintenance API via forwarder (talosctl version)..."
overlay_ok=1
for i in "${!CP_ARR[@]}"; do
  fwd="${CP_ARR[$i]}"; read -r host port <<< "$(parse_host_port "$fwd")"; real="$(real_ip_for "$host" "$port" "$i" cp)"
  echo "  checking control-plane node=$real via $host:$port"
  if ! talosctl version -e "$host:$port" -n "$real" -i --short >/dev/null 2>&1; then
    echo "    ERROR: talosctl couldn't reach $real via $host:$port (overlay route likely missing)"; overlay_ok=0
  else
    echo "    OK: talosctl maintenance API reachable"
  fi
done
[[ $overlay_ok -eq 1 ]] || exit 1
