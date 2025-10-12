#!/usr/bin/env bash
# Optional helper to configure and run a local Headscale server using Docker.
#
# This is intended for local/dev clusters. In production, deploy Headscale
# properly with HTTPS (behind a reverse proxy or ACME) and persistent storage.
#
# Requirements:
#  - docker
#
# Usage examples:
#   scripts/headscale-run.sh up                     # start container (127.0.0.1:8081)
#   scripts/headscale-run.sh status                 # show container status
#   scripts/headscale-run.sh down                   # stop & remove container
#   scripts/headscale-run.sh create-user myuser     # create a Headscale user
#   scripts/headscale-run.sh preauth-key myuser     # issue a pre-auth key
#
# Environment overrides:
#   HEADSCALE_STATE_DIR     default: $HOME/.guildnet/headscale
#   HEADSCALE_SERVER_URL    default: http://127.0.0.1:8081
#   HEADSCALE_IMAGE         default: ghcr.io/juanfont/headscale:0.22.3
#   HEADSCALE_CONTAINER_NAME default: guildnet-headscale
#   HEADSCALE_PORT          default: 8081 (host port -> container 8080)
#
set -euo pipefail

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need docker

STATE_DIR=${HEADSCALE_STATE_DIR:-"$HOME/.guildnet/headscale"}
CONF_DIR="$STATE_DIR/config"
DATA_DIR="$STATE_DIR/data"
CONFIG="$CONF_DIR/config.yaml"
IMAGE=${HEADSCALE_IMAGE:-"ghcr.io/juanfont/headscale:0.22.3"}
CONTAINER=${HEADSCALE_CONTAINER_NAME:-"guildnet-headscale"}

# Choose host bind address and port (auto-detect LAN IP; auto-bump port if busy when not explicitly set)
detect_lan_ip() {
  case "$(uname -s)" in
    Darwin)
      # Find default route interface, then its IPv4 address
      local ifc
      ifc=$(route -n get default 2>/dev/null | awk '/interface:/{print $2}' | head -n1)
      if [ -n "$ifc" ]; then ipconfig getifaddr "$ifc" 2>/dev/null || true; fi
      ;;
    Linux)
      ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}' | head -n1
      ;;
  esac
}

BIND_HOST=${HEADSCALE_BIND_HOST:-}
if [ -z "$BIND_HOST" ]; then
  BIND_HOST=$(detect_lan_ip || true)
  # Fallback to 0.0.0.0 if detection fails
  if [ -z "$BIND_HOST" ]; then BIND_HOST=0.0.0.0; fi
fi

# Choose host port (auto-bump if busy when not explicitly set)
DEFAULT_PORT=8081
if [ -n "${HEADSCALE_PORT:-}" ]; then
  HOST_PORT="$HEADSCALE_PORT"
else
  HOST_PORT="$DEFAULT_PORT"
  is_busy() {
    if command -v lsof >/dev/null 2>&1; then
      # Check on both 0.0.0.0 and specific bind host to be safe
      lsof -nP -iTCP:"$1" -sTCP:LISTEN -t >/dev/null 2>&1
    else
      nc -z 127.0.0.1 "$1" >/dev/null 2>&1 || nc -z "$BIND_HOST" "$1" >/dev/null 2>&1
    fi
  }
  tries=0; max=20
  while is_busy "$HOST_PORT"; do
    HOST_PORT=$((HOST_PORT+1))
    tries=$((tries+1))
    [ $tries -ge $max ] && { echo "[headscale] No free port found near $DEFAULT_PORT" >&2; exit 1; }
  done
fi

# Build default server URL unless overridden
if [ -n "${HEADSCALE_SERVER_URL:-}" ]; then
  SERVER_URL="$HEADSCALE_SERVER_URL"
else
  SERVER_URL="http://${BIND_HOST}:${HOST_PORT}"
fi

mkdir -p "$CONF_DIR" "$DATA_DIR"

write_default_config() {
  cat >"$CONFIG" <<EOF
server_url: ${SERVER_URL}
listen_addr: 0.0.0.0:8080
metrics_listen_addr: 127.0.0.1:9090
ip_prefixes:
  - 100.64.0.0/10
  - fd7a:115c:a1e0::/48
db_type: sqlite3
db_path: /var/lib/headscale/db.sqlite
private_key_path: /var/lib/headscale/server_private.key
log:
  level: info
  format: text
dns_config:
  override_local_dns: false
noise:
  private_key_path: /var/lib/headscale/noise_private.key
EOF
}

ensure_config() {
  CONFIG_CHANGED=0
  if [ ! -f "$CONFIG" ]; then
    echo "[headscale] Writing default config: $CONFIG"
    write_default_config
    CONFIG_CHANGED=1
  fi
  # Ensure required noise key path exists in config for recent Headscale versions
  if ! grep -q '^noise:' "$CONFIG"; then
    echo "[headscale] Adding required noise.private_key_path to config"
    printf "\nnoise:\n  private_key_path: /var/lib/headscale/noise_private.key\n" >> "$CONFIG"
    CONFIG_CHANGED=1
  fi
  # Ensure legacy server private key path exists for older versions
  if ! grep -q '^private_key_path:' "$CONFIG"; then
    echo "[headscale] Adding server private_key_path to config"
    printf "private_key_path: /var/lib/headscale/server_private.key\n" | cat - "$CONFIG" >"$CONFIG.tmp" && mv "$CONFIG.tmp" "$CONFIG"
    CONFIG_CHANGED=1
  fi
}

up() {
  ensure_config
  if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
    running=$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null || echo false)
    if [ "$running" = "true" ]; then
      if [ "${CONFIG_CHANGED:-0}" = "1" ]; then
        echo "[headscale] Config changed; restarting container."
        docker restart "$CONTAINER" >/dev/null
      else
        echo "[headscale] Container already running."
      fi
    else
      echo "[headscale] Recreating container $CONTAINER with current config."
      docker rm -f "$CONTAINER" >/dev/null || true
      echo "[headscale] Starting container $CONTAINER on ${BIND_HOST}:${HOST_PORT}"
      docker run -d \
        --name "$CONTAINER" \
        --restart unless-stopped \
        -p ${BIND_HOST}:${HOST_PORT}:8080 \
        -v "$DATA_DIR:/var/lib/headscale" \
        -v "$CONF_DIR:/etc/headscale:ro" \
        "$IMAGE" headscale serve >/dev/null
    fi
  else
    echo "[headscale] Starting container $CONTAINER on ${BIND_HOST}:${HOST_PORT}"
    docker run -d \
      --name "$CONTAINER" \
      --restart unless-stopped \
      -p ${BIND_HOST}:${HOST_PORT}:8080 \
      -v "$DATA_DIR:/var/lib/headscale" \
      -v "$CONF_DIR:/etc/headscale:ro" \
      "$IMAGE" headscale serve >/dev/null
  fi
  # Determine the actual mapped host:port for 8080/tcp
  MAPPED_HOST=$(docker inspect -f '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostIp }}' "$CONTAINER" 2>/dev/null || echo "")
  MAPPED_PORT=$(docker inspect -f '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort }}' "$CONTAINER" 2>/dev/null || echo "")
  if [ -n "$MAPPED_PORT" ]; then
    # If Docker binds to 0.0.0.0, prefer the detected LAN IP for a usable URL
    if [ "$MAPPED_HOST" = "0.0.0.0" ] || [ -z "$MAPPED_HOST" ]; then
      MAPPED_HOST=$(detect_lan_ip || echo 127.0.0.1)
    fi
    SERVER_URL="http://${MAPPED_HOST}:${MAPPED_PORT}"
  fi
  echo "[headscale] Server URL: ${SERVER_URL}"
  echo "[headscale] Data dir:  $STATE_DIR"
  # Persist the chosen URL for other scripts to consume
  printf "%s" "$SERVER_URL" > "$STATE_DIR/server_url"
}

down() {
  if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
    echo "[headscale] Stopping and removing $CONTAINER"
    docker rm -f "$CONTAINER" >/dev/null
  else
    echo "[headscale] Container not found: $CONTAINER"
  fi
}

status() {
  if docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' | grep -q "^${CONTAINER}\b"; then
    docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' | (head -n1; grep "^${CONTAINER}\b")
    MAPPED_HOST=$(docker inspect -f '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostIp }}' "$CONTAINER" 2>/dev/null || echo "")
    MAPPED_PORT=$(docker inspect -f '{{ (index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort }}' "$CONTAINER" 2>/dev/null || echo "")
    if [ -n "$MAPPED_PORT" ]; then
      if [ "$MAPPED_HOST" = "0.0.0.0" ] || [ -z "$MAPPED_HOST" ]; then
        MAPPED_HOST=$(detect_lan_ip || echo 127.0.0.1)
      fi
      echo "[headscale] Effective URL: http://${MAPPED_HOST}:${MAPPED_PORT}"
    fi
  else
    echo "[headscale] Not running. Use: $0 up"
  fi
}

create_user() {
  local user="${1:-}"; if [ -z "$user" ]; then echo "Usage: $0 create-user <name>" >&2; exit 2; fi
  docker exec -i "$CONTAINER" headscale users create "$user" || true
  echo "[headscale] Users:"; docker exec -i "$CONTAINER" headscale users list || true
}

preauth_key() {
  local user="${1:-}"; if [ -z "$user" ]; then echo "Usage: $0 preauth-key <user>" >&2; exit 2; fi
  docker exec -i "$CONTAINER" headscale preauthkeys create --reusable --ephemeral=false --expiration 24h --user "$user" | tee "$STATE_DIR/preauth-${user}.txt"
}

cmd="${1:-up}"; shift || true
case "$cmd" in
  up) up ;;
  down) down ;;
  status) status ;;
  create-user) create_user "$@" ;;
  preauth-key) preauth_key "$@" ;;
  *) echo "Unknown command: $cmd" >&2; exit 2 ;;
esac

cat <<INFO

Next steps:
- Create a user:    scripts/headscale-run.sh create-user myuser
- Create a key:     scripts/headscale-run.sh preauth-key myuser
- Use in host app:  ~/.guildnet/config.json -> login_server: ${SERVER_URL}
                    auth_key: (use the preauth key printed above)
                    hostname: (set a node name)

Notes:
- For production, put Headscale behind a proper TLS reverse proxy (or set ACME).
- Some Tailscale clients expect HTTPS on login_server. This helper uses HTTP on
  localhost for convenience; if you need HTTPS locally, front it with a proxy
  you trust and set login_server to that https:// URL.
INFO
