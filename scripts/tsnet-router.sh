#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

BIN="$ROOT/bin/tsnet-subnet-router"
STATE_DIR="$HOME/.guildnet/tsnet-router"
LOG="$STATE_DIR/router.log"
PIDFILE="$STATE_DIR/router.pid"

mkdir -p "$STATE_DIR"

ensure_bin() {
  if [ ! -x "$BIN" ]; then
    (cd "$ROOT" && make tsnet-subnet-router)
  fi
}

up() {
  ensure_bin
  if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; then
    echo "[tsnet-router] already running (pid $(cat "$PIDFILE"))"
    return 0
  fi
  ( set -a; [ -f "$ROOT/.env" ] && . "$ROOT/.env"; \
    TS_HOSTNAME="${TS_HOSTNAME:-host-app}-router" \
    nohup "$BIN" >"$LOG" 2>&1 & echo $! >"$PIDFILE" )
  echo "[tsnet-router] started (pid $(cat "$PIDFILE")) -> logs: $LOG"
}

down() {
  if [ -f "$PIDFILE" ]; then
    PID=$(cat "$PIDFILE" 2>/dev/null || echo "")
    if [ -n "$PID" ]; then kill "$PID" 2>/dev/null || true; fi
    rm -f "$PIDFILE"
    echo "[tsnet-router] stopped"
  else
    echo "[tsnet-router] not running"
  fi
}

status() {
  if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; then
    echo "[tsnet-router] running (pid $(cat "$PIDFILE"))"
    tail -n 20 "$LOG" 2>/dev/null || true
  else
    echo "[tsnet-router] not running"
  fi
}

logs() { tail -f "$LOG"; }

case "${1:-status}" in
  up) up ;;
  down) down ;;
  status) status ;;
  logs) logs ;;
  *) echo "Usage: $0 {up|down|status|logs}" >&2; exit 2 ;;
esac
