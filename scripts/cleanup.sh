#!/usr/bin/env bash
set -euo pipefail

# Cleanup script for GuildNet Host App
# - Stops running hostapp
# - Removes ~/.guildnet/state (tsnet identity)
# - Optionally clears logs and run files

CONF_DIR="$HOME/.guildnet"
RUN_DIR="$CONF_DIR/run"
LOG_DIR="$CONF_DIR/logs"
STATE_DIR="$CONF_DIR/state"
PID_FILE="$RUN_DIR/hostapp.pid"

stop_app() {
  if [ -s "$PID_FILE" ]; then
    pid=$(cat "$PID_FILE" || true)
    if [ -n "$pid" ] && kill -0 "$pid" >/dev/null 2>&1; then
      echo "Stopping hostapp (pid=$pid)"
      kill "$pid" || true
      sleep 1
    fi
    rm -f "$PID_FILE"
  fi
}

case "${1:-}" in
  --all)
    stop_app
    echo "Removing state: $STATE_DIR"
    rm -rf "$STATE_DIR"
    echo "Clearing logs: $LOG_DIR"
    rm -f "$LOG_DIR"/*.log || true
    ;;
  *)
    stop_app
    echo "Cleanup done. Use '--all' to also remove state and logs."
    ;;
esac
