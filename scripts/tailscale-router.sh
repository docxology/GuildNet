#!/usr/bin/env bash
# Host-level Tailscale subnet router helper (Option A)
# - Installs and runs the native Tailscale client as a subnet router
# - Advertises TS_ROUTES (defaults include 10.0.0.0/24) and accepts routes
# - Works with Tailscale SaaS or Headscale (TS_LOGIN_SERVER)
#
# Usage:
#   scripts/tailscale-router.sh up
#   scripts/tailscale-router.sh down
#   scripts/tailscale-router.sh status
#   scripts/tailscale-router.sh install    # install tailscale client
#
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# Load shared env if present
if [ -f "$ROOT/.env" ]; then
  # shellcheck disable=SC1090
  . "$ROOT/.env"
fi

# Defaults
TS_AUTHKEY=${TS_AUTHKEY:-}
TS_LOGIN_SERVER=${TS_LOGIN_SERVER:-${HEADSCALE_URL:-https://login.tailscale.com}}
TS_ROUTES=${TS_ROUTES:-10.0.0.0/24,10.96.0.0/12,10.244.0.0/16}
TS_HOSTNAME=${ROUTER_HOSTNAME:-${TS_HOSTNAME:-guildnet-router}}

need() { command -v "$1" >/dev/null 2>&1; }
log() { printf "%s\n" "$*"; }
err() { printf "ERR: %s\n" "$*" >&2; }

install() {
  if need tailscale && need tailscaled; then
    log "tailscale already installed"
    return
  fi
  case "$(uname -s)" in
    Darwin)
      if ! need brew; then err "Homebrew is required to install tailscale on macOS. https://brew.sh"; exit 1; fi
      brew install tailscale ;;
    Linux)
      # Use official cross-distro install script for reliability
      curl -fsSL https://tailscale.com/install.sh | sh ;;
    *) err "Unsupported OS: $(uname -s)"; exit 1 ;;
  esac
}

start() {
  if ! need tailscaled; then err "tailscaled not installed; run: scripts/tailscale-router.sh install"; exit 1; fi
  # Start daemon if not running
  if command -v systemctl >/dev/null 2>&1; then
    # If running as root, operate on system service directly.
    if [ "$(id -u)" -eq 0 ]; then
      systemctl enable --now tailscaled 2>/dev/null || true
    else
      # Try user/system, then sudo (non-interactive) as fallback
      systemctl --user enable --now tailscaled 2>/dev/null || true
      systemctl enable --now tailscaled 2>/dev/null || sudo -n systemctl enable --now tailscaled 2>/dev/null || true
    fi
  elif command -v service >/dev/null 2>&1; then
    if [ "$(id -u)" -eq 0 ]; then
      service tailscaled start || true
    else
      service tailscaled start || sudo -n service tailscaled start || true
    fi
  else
    # macOS: try brew services first, otherwise run tailscaled in user space
    if command -v brew >/dev/null 2>&1; then
      brew services start tailscale || true
    fi
    # If still not running, start a user-space tailscaled without sudo
    if ! tailscale status >/dev/null 2>&1; then
      STATE_DIR="$HOME/Library/Application Support/tsnet-router"
      mkdir -p "$STATE_DIR"
      nohup tailscaled --state="$STATE_DIR/tailscaled.state" >/tmp/tailscaled.router.log 2>&1 &
      for i in $(seq 1 15); do if tailscale status >/dev/null 2>&1; then break; fi; sleep 1; done
    fi
  fi
}

up() {
  if [ -z "$TS_AUTHKEY" ]; then err "TS_AUTHKEY not set (.env)."; exit 1; fi
  if printf '%s' "$TS_LOGIN_SERVER" | grep -qE '^https?://127\.0\.0\.1'; then
    err "TS_LOGIN_SERVER points to 127.0.0.1 which may not be reachable from this router host. Use a reachable URL."
  fi
  start
  # If running as root (sudo make ...), prefer running tailscale directly and set operator
  OP_USER=${SUDO_USER:-${USER:-root}}
  if [ "$(id -u)" -eq 0 ]; then
    # Ensure service is running as system service
    systemctl enable --now tailscaled 2>/dev/null || true
    # Try to set operator (best-effort)
    tailscale set --operator="$OP_USER" >/dev/null 2>&1 || true
    if tailscale up \
      --reset \
      --authkey="$TS_AUTHKEY" \
      --login-server="$TS_LOGIN_SERVER" \
      --advertise-routes="$TS_ROUTES" \
      --hostname="$TS_HOSTNAME" \
      --accept-routes \
      --accept-dns=false \
      --timeout=60s; then
      return 0
    fi
    err "tailscale up failed when running as root. Ensure the daemon is running and the auth key is valid."
    exit 1
  fi

  # Try without sudo first for non-root users
  if tailscale up \
    --reset \
    --authkey="$TS_AUTHKEY" \
    --login-server="$TS_LOGIN_SERVER" \
    --advertise-routes="$TS_ROUTES" \
    --hostname="$TS_HOSTNAME" \
    --accept-routes \
    --accept-dns=false \
    --timeout=60s; then
    return 0
  fi

  # Try non-interactive sudo for non-root
  if command -v sudo >/dev/null 2>&1; then
    sudo -n tailscale set --operator="$USER" >/dev/null 2>&1 || true
    if sudo -n tailscale up \
      --reset \
      --authkey="$TS_AUTHKEY" \
      --login-server="$TS_LOGIN_SERVER" \
      --advertise-routes="$TS_ROUTES" \
      --hostname="$TS_HOSTNAME" \
      --accept-routes \
      --accept-dns=false \
      --timeout=60s; then
      return 0
    fi
    echo "sudo privilege required to complete 'tailscale up' (will prompt)." >&2
    # Interactive sudo fallback (one-time)
    if sudo tailscale set --operator="$USER" 2>/dev/null || true; then :; fi
    if sudo tailscale up \
      --reset \
      --authkey="$TS_AUTHKEY" \
      --login-server="$TS_LOGIN_SERVER" \
      --advertise-routes="$TS_ROUTES" \
      --hostname="$TS_HOSTNAME" \
      --accept-routes \
      --accept-dns=false \
      --timeout=60s; then
      return 0
    fi
  fi
  err "tailscale up failed. Ensure the daemon is running and the auth key is valid."
  exit 1
}

down() {
  if need tailscale; then
    sudo -n tailscale down || tailscale down || true
  fi
}

status() {
  if need tailscale; then
    tailscale status || true
  else
    err "tailscale CLI not found; run install"
    return 1
  fi
}

cmd=${1:-status}
case "$cmd" in
  install) install ;;
  up) up ;;
  down) down ;;
  status) status ;;
  *) echo "Usage: $0 {install|up|down|status}" >&2; exit 2 ;;
esac
