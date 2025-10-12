#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
[ -f ./.env ] && . ./.env || true

if ! docker ps --format '{{.Names}}' | grep -q '^guildnet-headscale$'; then
  echo "Headscale container not running (guildnet-headscale)" >&2
  exit 1
fi

ROUTER_NAME="${ROUTER_HOSTNAME:-${TS_HOSTNAME:-host-app}-router}"

enable_ids() {
  local ids="$1"
  if [ -z "$ids" ]; then
    echo "No route IDs to enable for '$ROUTER_NAME'" >&2
    return 1
  fi
  for id in $ids; do
    echo "Enabling route id=$id for '$ROUTER_NAME' ..."
    docker exec -i guildnet-headscale headscale routes enable -r "$id" || true
    sleep 0.1
  done
}

# Try JSON parsing first (newer headscale)
if command -v jq >/dev/null 2>&1; then
  if JSON=$(docker exec -i guildnet-headscale headscale routes list -o json 2>/dev/null || true); then
    if [ -n "$JSON" ] && printf '%s' "$JSON" | jq -e . >/dev/null 2>&1; then
      IDS=$(printf '%s' "$JSON" | jq -r --arg m "$ROUTER_NAME" '.[] | select(.Machine==$m) | .ID' | tr -cd '0-9\n')
      if [ -n "$IDS" ]; then
        enable_ids "$IDS" || true
        echo "Current routes after enable:"; docker exec -i guildnet-headscale headscale routes list || true
        exit 0
      fi
    fi
  fi
fi

# Fallback: parse table output with colors disabled
RAW=$(docker exec -e CLICOLOR=0 -e NO_COLOR=1 -i guildnet-headscale headscale routes list 2>/dev/null || true)
# Extract IDs where Machine column matches
IDS=$(printf '%s' "$RAW" | awk -F '|' -v n="$ROUTER_NAME" 'NR>2 {id=$1; machine=$2; gsub(/^[ \t]+|[ \t]+$/, "", id); gsub(/^[ \t]+|[ \t]+$/, "", machine); if (machine==n) print id}' | tr -cd '0-9\n')
if [ -z "$IDS" ]; then
  echo "No routes found for machine '$ROUTER_NAME'. Ensure the router is up and advertising routes." >&2
  printf "%s\n" "$RAW" || true
  exit 4
fi

enable_ids "$IDS" || true

echo "Current routes after enable:"
docker exec -i guildnet-headscale headscale routes list || true
