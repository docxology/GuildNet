#!/usr/bin/env bash
set -euo pipefail

# Verify the share-and-join flow in isolation:
# - Builds the binary
# - Creates a join config using create_join_info.sh
# - Spawns a temp HOME
# - Runs join.sh with that config and verifies Host App health
#
# Usage examples:
#   scripts/verify_join.sh \
#     --hostapp-url https://myhost.tail:443 \
#     --include-ca certs/server.crt \
#     --login-server https://headscale.example.com \
#     --auth-key tskey-abc123

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
  cat <<USAGE
Usage: scripts/verify_join.sh --hostapp-url URL [--include-ca PATH] [--login-server URL] [--auth-key KEY] [--hostname NAME] [--name LABEL]

This script does not start a Host App; it verifies the join workflow against an existing Host App URL.
USAGE
}

HOSTAPP_URL=""
INCLUDE_CA=""
LOGIN_SERVER=""
AUTH_KEY=""
HOSTNAME=""
NAME_LABEL=""

while [ $# -gt 0 ]; do
  case "$1" in
    --hostapp-url) HOSTAPP_URL="${2:-}"; shift 2 ;;
    --include-ca) INCLUDE_CA="${2:-}"; shift 2 ;;
    --login-server) LOGIN_SERVER="${2:-}"; shift 2 ;;
    --auth-key) AUTH_KEY="${2:-}"; shift 2 ;;
    --hostname) HOSTNAME="${2:-}"; shift 2 ;;
    --name) NAME_LABEL="${2:-}"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown arg: $1" >&2; usage; exit 2 ;;
  esac
done

if [ -z "$HOSTAPP_URL" ]; then echo "ERROR: --hostapp-url required" >&2; exit 2; fi

# Dependencies
need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
for c in jq curl; do need "$c"; done

# Build (binary may be useful but not strictly required for join)
if grep -q "^build:" "$REPO_ROOT/Makefile" 2>/dev/null; then
  (cd "$REPO_ROOT" && make build)
else
  (cd "$REPO_ROOT" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/hostapp ./cmd/hostapp)
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

JOIN_CFG="$TMP_DIR/guildnet.config"

# Create the join info
CMD=("$SCRIPT_DIR/create_join_info.sh" --hostapp-url "$HOSTAPP_URL" --out "$JOIN_CFG")
[ -n "$INCLUDE_CA" ] && CMD+=(--include-ca "$INCLUDE_CA")
[ -n "$LOGIN_SERVER" ] && CMD+=(--login-server "$LOGIN_SERVER")
[ -n "$AUTH_KEY" ] && CMD+=(--auth-key "$AUTH_KEY")
[ -n "$HOSTNAME" ] && CMD+=(--hostname "$HOSTNAME")
[ -n "$NAME_LABEL" ] && CMD+=(--name "$NAME_LABEL")

"${CMD[@]}"
[ -s "$JOIN_CFG" ] || { echo "ERROR: failed to produce $JOIN_CFG" >&2; exit 1; }

# Isolated HOME
ISO_HOME="$TMP_DIR/home"
mkdir -p "$ISO_HOME"
export HOME="$ISO_HOME"

# Run join
"$SCRIPT_DIR/join.sh" "$JOIN_CFG" --non-interactive

# Validate outputs
CONF_FILE="$HOME/.guildnet/config.json"
if [ -s "$CONF_FILE" ]; then
  echo "OK: wrote $CONF_FILE" >&2
else
  echo "NOTE: no tsnet config written (likely no pre-auth in join file)" >&2
fi

# Health check against provided URL
CA_OPT=()
if [ -n "$INCLUDE_CA" ]; then CA_OPT=(--cacert "$INCLUDE_CA"); else CA_OPT=(-k); fi
if curl -sS "${CA_OPT[@]}" "$HOSTAPP_URL/healthz" | grep -q "ok"; then
  echo "OK: Host App health verified at $HOSTAPP_URL" >&2
else
  echo "ERROR: Host App health failed at $HOSTAPP_URL" >&2
  exit 1
fi

echo "Verify join: SUCCESS" >&2
