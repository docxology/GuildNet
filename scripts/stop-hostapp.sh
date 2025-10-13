#!/usr/bin/env bash
set -euo pipefail

# Stop hostapp processes listening on the configured LISTEN_LOCAL port.
# Usage: LISTEN_LOCAL=host:port ./scripts/stop-hostapp.sh

LISTEN_LOCAL=${LISTEN_LOCAL:-127.0.0.1:8090}

PORT_PART=${LISTEN_LOCAL##*:}
if ! [[ "$PORT_PART" =~ ^[0-9]+$ ]]; then
  echo "Invalid LISTEN_LOCAL (expected host:port): $LISTEN_LOCAL" >&2
  exit 2
fi

PIDS=$(ss -ltnp 2>/dev/null | awk '/LISTEN/ {print}' | grep -E ":${PORT_PART}\\b" || true)
if [ -z "$PIDS" ]; then
  echo "stop-hostapp: no listener on port ${PORT_PART}"
  exit 0
fi

PIDS=$(printf "%s\n" "$PIDS" | sed -n 's/.*pid=\([0-9]\+\).*/\1/p' | sort -u)

found=0
for pid in $PIDS; do
  exe=$(readlink "/proc/$pid/exe" 2>/dev/null || true)
  if echo "$exe" | grep -q "/hostapp$"; then
    echo "stop-hostapp: stopping hostapp pid=$pid"
    kill -INT "$pid" 2>/dev/null || true
    found=1
    continue
  fi
  # fallback: check cmdline
  if tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -q "hostapp"; then
    echo "stop-hostapp: stopping hostapp pid=$pid (cmdline match)"
    kill -INT "$pid" 2>/dev/null || true
    found=1
    continue
  fi
  echo "stop-hostapp: port ${PORT_PART} in use by pid=$pid (not hostapp); leaving it alone"
done

if [ "$found" -eq 1 ]; then
  # wait briefly and force kill if needed
  end=$((SECONDS + 5))
  for pid in $PIDS; do
    while kill -0 "$pid" 2>/dev/null && [ $SECONDS -lt $end ]; do
      sleep 0.2
    done
    if kill -0 "$pid" 2>/dev/null; then
      echo "stop-hostapp: force killing pid=$pid"
      kill -KILL "$pid" 2>/dev/null || true
    fi
  done
  echo "stop-hostapp: done"
else
  echo "stop-hostapp: no hostapp instances found on port ${PORT_PART}"
fi
