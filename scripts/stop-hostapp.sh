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
# First attempt graceful shutdown via internal HTTP endpoint
SHUT_HOST=${LISTEN_LOCAL%%:*}
SHUT_PORT=${LISTEN_LOCAL##*:}
if command -v curl >/dev/null 2>&1; then
  echo "stop-hostapp: attempting HTTP shutdown on ${SHUT_HOST}:${SHUT_PORT}";
  curl --http1.1 -k -s -m 2 -X POST "https://${SHUT_HOST}:${SHUT_PORT}/internal/shutdown" || true
  # give it a moment
  sleep 1
  # re-evaluate PIDs
  PIDS=$(ss -ltnp 2>/dev/null | awk '/LISTEN/ {print}' | grep -E ":${PORT_PART}\\b" || true)
  PIDS=$(printf "%s\n" "$PIDS" | sed -n 's/.*pid=\([0-9]\+\).*/\1/p' | sort -u)
fi
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
  # Also check for launcher parents and stop them to avoid immediate restart
  for pid in $PIDS; do
    # get parent pid
    ppid=$(awk '{print $4}' /proc/$pid/stat 2>/dev/null || true)
    if [ -n "$ppid" ] && [ "$ppid" -ne 1 ]; then
      # inspect parent's cmdline
      parent_cmd=$(tr '\0' ' ' < /proc/$ppid/cmdline 2>/dev/null || true)
      if echo "$parent_cmd" | grep -q "run-hostapp.sh\|run-hostapp"; then
        echo "stop-hostapp: stopping launcher parent pid=$ppid (cmdline match)"
        kill -INT "$ppid" 2>/dev/null || true
        # give it a moment and then force kill if still alive
        end2=$((SECONDS + 3))
        while kill -0 "$ppid" 2>/dev/null && [ $SECONDS -lt $end2 ]; do
          sleep 0.2
        done
        if kill -0 "$ppid" 2>/dev/null; then
          echo "stop-hostapp: force killing launcher parent pid=$ppid"
          kill -KILL "$ppid" 2>/dev/null || true
        fi
      fi
    fi
  done
  echo "stop-hostapp: done"
else
  echo "stop-hostapp: no hostapp instances found on port ${PORT_PART}"
fi
