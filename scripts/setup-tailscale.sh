#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

# 1) Ensure forwarding (may prompt for sudo)
bash "$ROOT/scripts/enable-ip-forwarding.sh" || true

# 2) Install and bring up host tailscale router
make router-install || true
make router-up
make router-status || true

# 3) Approve routes in Headscale (best-effort)
if docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$'; then
  ROUTER_NAME=$(bash -lc 'set -a; [ -f ./.env ] && . ./.env; echo "${ROUTER_HOSTNAME:-${TS_HOSTNAME:-host-app}-router}"')
  RID=$(docker exec -i guildnet-headscale headscale machines list | awk -v n="$ROUTER_NAME" '$0 ~ n {print $1; exit}')
  if [ -n "${RID:-}" ]; then
    docker exec -i guildnet-headscale headscale routes enable -r "$RID" || true
  fi
  docker exec -i guildnet-headscale headscale routes list || true
fi

echo "Tailscale router setup complete."