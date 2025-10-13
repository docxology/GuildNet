#!/usr/bin/env bash
set -euo pipefail

# Simple launcher for GuildNet HostApp (no supervision, no DB lock handling)
ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BIN="${ROOT_DIR}/bin/hostapp"

# Prefer GuildNet kubeconfig for hostapp if present
if [ -f "$HOME/.guildnet/kubeconfig" ]; then
  export KUBECONFIG="${KUBECONFIG:-$HOME/.guildnet/kubeconfig}"
fi

# Determine listen address (must match server logic): env LISTEN_LOCAL or default 127.0.0.1:8090
LISTEN_LOCAL="${LISTEN_LOCAL:-}"
if [ -z "$LISTEN_LOCAL" ]; then
  LISTEN_LOCAL="127.0.0.1:8090"
fi

# Extract port from LISTEN_LOCAL
HOST_PART=${LISTEN_LOCAL%:*}
PORT_PART=${LISTEN_LOCAL##*:}
if ! [[ "$PORT_PART" =~ ^[0-9]+$ ]]; then
  echo "Invalid LISTEN_LOCAL (expected host:port): $LISTEN_LOCAL" >&2
  exit 2
fi

# If an existing hostapp is listening on this port (v4 or v6), terminate it first
existing_pids=()
if command -v ss >/dev/null 2>&1; then
  # Collect PIDs from ss output for listeners on the target port
  while read -r line; do
    pid=$(echo "$line" | sed -n 's/.*pid=\([0-9]\+\).*/\1/p' | head -n1)
    [ -n "$pid" ] && existing_pids+=("$pid")
  done < <(ss -ltnp 2>/dev/null | awk '/LISTEN/ {print}' | grep -E ":${PORT_PART}\\b" || true)
fi

# De-dup PIDs
if [ ${#existing_pids[@]} -gt 0 ]; then
  mapfile -t existing_pids < <(printf "%s\n" "${existing_pids[@]}" | awk 'NF' | sort -u)
fi

kill_count=0
for pid in "${existing_pids[@]}"; do
  # Confirm the process is a hostapp instance
  exe=$(readlink "/proc/$pid/exe" 2>/dev/null || true)
  if echo "$exe" | grep -q "/hostapp$"; then
    echo "Stopping existing hostapp (pid=$pid) on port $PORT_PART..."
    kill "$pid" 2>/dev/null || true
    kill_count=$((kill_count+1))
  else
    # Fallback check cmdline
    if tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -q "hostapp"; then
      echo "Stopping existing hostapp (pid=$pid) on port $PORT_PART..."
      kill "$pid" 2>/dev/null || true
      kill_count=$((kill_count+1))
    fi
  fi
done

if [ "$kill_count" -gt 0 ]; then
  # Wait briefly for processes to exit; force kill if needed
  end=$((SECONDS + 5))
  while [ $SECONDS -lt $end ]; do
    alive=0
    for pid in "${existing_pids[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then alive=1; fi
    done
    [ $alive -eq 0 ] && break
    sleep 0.2
  done
  for pid in "${existing_pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      echo "Force killing hostapp (pid=$pid)"
      kill -KILL "$pid" 2>/dev/null || true
    fi
  done
fi

# As a safeguard, if a non-hostapp process is still listening on the port, abort
if ss -ltnp 2>/dev/null | awk '/LISTEN/ {print}' | grep -E ":${PORT_PART}\\b" >/dev/null 2>&1; then
  # Re-check whether any listener is ours; if none were ours, refuse to start
  listeners=$(ss -ltnp 2>/dev/null | awk '/LISTEN/ {print}' | grep -E ":${PORT_PART}\\b" | sed -n 's/.*pid=\([0-9]\+\).*/\1/p' | sort -u)
  ours=0
  for pid in $listeners; do
    exe=$(readlink "/proc/$pid/exe" 2>/dev/null || true)
    if echo "$exe" | grep -q "/hostapp$"; then ours=1; fi
  done
  if [ $ours -eq 0 ]; then
    echo "Port ${PORT_PART} is in use by another process; aborting." >&2
    ss -ltnp | grep -E ":${PORT_PART}\\b" || true
    exit 1
  fi
fi

exec "$BIN" serve
