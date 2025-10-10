#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

detect_lan_ip() {
  case "$(uname -s)" in
    Darwin)
      local ifc
      ifc=$(route -n get default 2>/dev/null | awk '/interface:/{print $2}' | head -n1)
      if [ -n "$ifc" ]; then ipconfig getifaddr "$ifc" 2>/dev/null || true; fi
      ;;
    Linux)
      ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}' | head -n1
      ;;
  esac
}

ENV_FILE="$ROOT/.env"
if [ ! -f "$ENV_FILE" ]; then
  echo ".env not found at $ENV_FILE" >&2
  exit 1
fi

# Prefer the headscale helper's persisted URL if present; else derive from LAN IP
# Prefer container mapping if container is present
PREFERRED_URL=""
if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -q '^guildnet-headscale$'; then
  MAPPED_HOST=$(docker inspect -f '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostIp }}' guildnet-headscale 2>/dev/null || echo "")
  MAPPED_PORT=$(docker inspect -f '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort }}' guildnet-headscale 2>/dev/null || echo "")
  if [ -n "$MAPPED_PORT" ]; then
    if [ "$MAPPED_HOST" = "0.0.0.0" ] || [ -z "$MAPPED_HOST" ]; then
      MAPPED_HOST=$(detect_lan_ip || echo 127.0.0.1)
    fi
    PREFERRED_URL="http://${MAPPED_HOST}:${MAPPED_PORT}"
  fi
fi
if [ -z "$PREFERRED_URL" ]; then
  STATE_DIR="$HOME/.guildnet/headscale"
  SERVER_FILE="$STATE_DIR/server_url"
  if [ -f "$SERVER_FILE" ]; then
    PREFERRED_URL=$(cat "$SERVER_FILE" | tr -d '\n\r')
  fi
fi
if [ -z "$PREFERRED_URL" ]; then
  LAN_IP=$(detect_lan_ip || true)
  if [ -n "$LAN_IP" ]; then
    PREFERRED_URL="http://$LAN_IP:8081"
  fi
fi

if [ -n "$PREFERRED_URL" ]; then
  tmp=$(mktemp)
  sed -E "s#^(TS_LOGIN_SERVER=).*#\1$PREFERRED_URL#" "$ENV_FILE" > "$tmp" && mv "$tmp" "$ENV_FILE"
  tmp=$(mktemp)
  sed -E "s#^(HEADSCALE_URL=).*#\1$PREFERRED_URL#" "$ENV_FILE" > "$tmp" && mv "$tmp" "$ENV_FILE"
  echo "Synchronized TS_LOGIN_SERVER and HEADSCALE_URL to $PREFERRED_URL."
else
  echo "No preferred Headscale URL detected; leaving .env unchanged."
fi
