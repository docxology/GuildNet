#!/usr/bin/env bash
set -euo pipefail
# Helper to open an SSH local port-forward from localhost:LOCAL_PORT to REMOTE_IP:6443
# Usage: SSH_TUNNEL_HOST=host SSH_TUNNEL_USER=user ./scripts/tunnel-kubeapi.sh start
#        ./scripts/tunnel-kubeapi.sh stop

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PIDFILE="$ROOT/tmp/tunnel-kubeapi.pid"
LOCAL_PORT=${LOCAL_PORT:-6443}
REMOTE_IP=${REMOTE_IP:-10.0.0.10}

start() {
  : "Starting SSH tunnel ${SSH_TUNNEL_USER}@${SSH_TUNNEL_HOST}:${LOCAL_PORT}->${REMOTE_IP}:6443"
  mkdir -p "$(dirname "$PIDFILE")"
  # Background ssh; -o ExitOnForwardFailure to ensure it fails if port can't be forwarded
  ssh -o ExitOnForwardFailure=yes -f -N -L "${LOCAL_PORT}:${REMOTE_IP}:6443" "${SSH_TUNNEL_USER}@${SSH_TUNNEL_HOST}"
  # Grab PID of background ssh (best-effort)
  sleep 0.2
  p=$(pgrep -f "ssh .*${LOCAL_PORT}:${REMOTE_IP}:6443" | head -n1 || true)
  if [ -n "$p" ]; then
    echo "$p" > "$PIDFILE"
    echo $p
    return 0
  fi
  echo "failed to start ssh tunnel" >&2
  return 2
}

stop() {
  if [ -f "$PIDFILE" ]; then
    pid=$(cat "$PIDFILE")
    if kill "$pid" >/dev/null 2>&1; then
      rm -f "$PIDFILE"
      echo "stopped $pid"
      return 0
    fi
  fi
  echo "no tunnel pidfile" >&2
  return 1
}

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  *) echo "Usage: $0 {start|stop} (requires SSH_TUNNEL_HOST and SSH_TUNNEL_USER env vars)" >&2; exit 2 ;;
esac
