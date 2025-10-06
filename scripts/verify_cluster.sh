#!/usr/bin/env bash
set -euo pipefail

# Portable verifier for GuildNet Host App (tsnet)
# - Builds the Go binary
# - Ensures ~/.guildnet/config.json exists (interactive wizard if needed)
# - Starts the server, waits for /healthz
# - Verifies /api/ping to Talos node and optional /proxy to a Service

# Colors
if [ -t 1 ]; then
  RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'
else
  RED=''; GREEN=''; YELLOW=''; BLUE=''; BOLD=''; NC=''
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
APP_BIN="$REPO_ROOT/bin/hostapp"
CONF_DIR="$HOME/.guildnet"
CONF_FILE="$CONF_DIR/config.json"
STATE_DIR="$CONF_DIR/state"
RUN_DIR="$CONF_DIR/run"
LOG_DIR="$CONF_DIR/logs"
PID_FILE="$RUN_DIR/hostapp.pid"
LOG_FILE="$LOG_DIR/hostapp.log"

usage() {
  cat <<USAGE
Usage: scripts/verify_cluster.sh [--talos-ip 100.x.y.z] [--svc <ip:port>] [--wait 40] [--no-restart]

Options:
  --talos-ip IP     Tailnet IP of a Talos node exposing TCP :50000.
  --svc ip:port     Optional ClusterIP:port or NodePort ip:port to test /proxy.
  --wait seconds    Time to wait for server readiness (default: 40).
  --no-restart      If server is running, do not restart it.
  --help            Show this help.

Examples:
  scripts/verify_cluster.sh --talos-ip 100.64.1.10
  scripts/verify_cluster.sh --talos-ip 100.64.1.10 --svc 10.96.0.200:8080
USAGE
}

# Defaults
TALOS_IP=""
SVC_ADDR=""
WAIT_SECS=40
NO_RESTART=0

# Parse args
while [ $# -gt 0 ]; do
  case "$1" in
    --talos-ip) TALOS_IP="${2:-}"; shift 2;;
    --svc) SVC_ADDR="${2:-}"; shift 2;;
    --wait) WAIT_SECS="${2:-}"; shift 2;;
    --no-restart) NO_RESTART=1; shift;;
    --help|-h) usage; exit 0;;
    *) echo -e "${YELLOW}WARN${NC}: unknown arg: $1"; shift;;
  esac
done

# Dependency checks
need_cmd() { command -v "$1" >/dev/null 2>&1 || return 1; }
ENSURE_MSG="Please install it. On macOS: brew install <pkg>; on Debian/Ubuntu: sudo apt-get install <pkg>"

MISSING=()
for c in go curl jq sed awk lsof; do
  need_cmd "$c" || MISSING+=("$c")
done
if [ ${#MISSING[@]} -gt 0 ]; then
  echo -e "${RED}FAIL${NC}: missing required commands: ${MISSING[*]}"
  echo "Install hints: $ENSURE_MSG"
  exit 1
fi

# timeout vs gtimeout
if command -v gtimeout >/dev/null 2>&1; then TIMEOUT=gtimeout; else TIMEOUT=timeout; fi
if ! command -v "$TIMEOUT" >/dev/null 2>&1; then
  echo -e "${YELLOW}WARN${NC}: timeout not found; install coreutils (macOS: brew install coreutils) or ensure 'timeout' available"
  # continue without TIMEOUT (we'll rely on curl --max-time where possible)
  TIMEOUT=""
fi

# Ensure directories
mkdir -p "$LOG_DIR" "$RUN_DIR" "$STATE_DIR"

# Build
if grep -q "^build:" "$REPO_ROOT/Makefile" 2>/dev/null; then
  echo -e "${BLUE}INFO${NC}: Building via make build"
  (cd "$REPO_ROOT" && make build)
else
  echo -e "${BLUE}INFO${NC}: Building via go build"
  (cd "$REPO_ROOT" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$APP_BIN" ./cmd/hostapp)
fi
if [ ! -x "$APP_BIN" ]; then echo -e "${RED}FAIL${NC}: build did not produce $APP_BIN"; exit 1; fi
echo -e "${GREEN}OK${NC}: Built $APP_BIN"

# Interactive prompts if not provided
prompt() { # $1 prompt, $2 default -> echoes value
  local p="$1" d="$2" ans
  read -r -p "$p [$d]: " ans || true
  if [ -z "$ans" ]; then echo "$d"; else echo "$ans"; fi
}

# Init config if missing/empty
if [ ! -s "$CONF_FILE" ]; then
  echo -e "${YELLOW}WARN${NC}: Config $CONF_FILE missing, starting init wizard..."
  echo "You'll need: Headscale login URL, pre-auth key, hostname, listen address, and allowlist."
  "$APP_BIN" init || { echo -e "${RED}FAIL${NC}: init wizard failed"; exit 1; }
fi

# Validate config JSON and required keys
if ! jq -e . "$CONF_FILE" >/dev/null 2>&1; then
  echo -e "${RED}FAIL${NC}: $CONF_FILE is not valid JSON"; exit 1
fi
req_keys='["login_server","auth_key","hostname","listen_local","dial_timeout_ms","allowlist"]'
missing=$(jq -r --argjson req "$req_keys" '($req - (paths(scalars)|map(tostring)|select(length==1)|.[0]) )|.[]?' "$CONF_FILE" 2>/dev/null || true)
# Simple existence check
for k in login_server auth_key hostname listen_local dial_timeout_ms allowlist; do
  if ! jq -e ".$k" "$CONF_FILE" >/dev/null 2>&1; then
    echo -e "${RED}FAIL${NC}: missing key '$k' in $CONF_FILE"; exit 1
  fi
done

LISTEN_LOCAL=$(jq -r .listen_local "$CONF_FILE")
BASE_URL="http://$LISTEN_LOCAL"

# Failure helper with next steps
die() {
  echo -e "${RED}FAIL${NC}: $*"
  echo "Next steps:"
  echo "  - Verify Headscale login_server URL in $CONF_FILE"
  echo "  - Check auth_key is valid and not expired"
  echo "  - Ensure allowlist contains needed entries (Talos IP:50000 and service ip:port)"
  echo "  - Confirm a Tailscale subnet router advertises Pod/Service CIDRs"
  exit 1
}

# Ensure allowlist entries
ensure_allowlist() {
  local addr="$1"
  if [ -z "$addr" ]; then return 0; fi
  local tmp
  tmp=$(mktemp)
  # Create allowlist if not array
  if ! jq -e '.allowlist and (.allowlist|type=="array")' "$CONF_FILE" >/dev/null; then
    cp "$CONF_FILE" "$CONF_FILE.bak" || true
    jq '.allowlist = []' "$CONF_FILE" > "$tmp" && mv "$tmp" "$CONF_FILE"
  fi
  if ! jq -e --arg a "$addr" '.allowlist|index($a)' "$CONF_FILE" >/dev/null; then
    cp "$CONF_FILE" "$CONF_FILE.bak" || true
    jq --arg a "$addr" '.allowlist = (.allowlist + [$a] | unique)' "$CONF_FILE" > "$tmp" && mv "$tmp" "$CONF_FILE"
    echo -e "${BLUE}INFO${NC}: Added '$addr' to allowlist"
  fi
}

# Parse/collect inputs
if [ -z "$TALOS_IP" ]; then TALOS_IP=$(prompt "Enter Talos node tailnet IP" ""); fi
if [ -z "$TALOS_IP" ]; then die "--talos-ip is required"; fi
ensure_allowlist "$TALOS_IP:50000"
if [ -n "$SVC_ADDR" ]; then ensure_allowlist "$SVC_ADDR"; fi

# Start/restart app
running_pid=""
if [ -s "$PID_FILE" ]; then
  pid=$(cat "$PID_FILE" || true)
  if [ -n "$pid" ] && kill -0 "$pid" >/dev/null 2>&1; then
    running_pid="$pid"
  fi
fi

if [ -n "$running_pid" ]; then
  echo -e "${YELLOW}WARN${NC}: hostapp already running (pid=$running_pid)"
  if [ "$NO_RESTART" -eq 1 ]; then
    echo -e "${BLUE}INFO${NC}: Reusing running instance"
  else
    ans=$(prompt "Restart it? (y/n)" "y")
    if [ "$ans" = "y" ] || [ "$ans" = "Y" ]; then
      echo -e "${BLUE}INFO${NC}: Stopping pid=$running_pid"
      kill "$running_pid" || true
      sleep 1
    else
      echo -e "${BLUE}INFO${NC}: Reusing running instance"
    fi
  fi
fi

# Start if not running
if ! { [ -n "$running_pid" ] && kill -0 "$running_pid" >/dev/null 2>&1; }; then
  echo -e "${BLUE}INFO${NC}: Starting hostapp (logs: $LOG_FILE)"
  nohup "$APP_BIN" serve >>"$LOG_FILE" 2>&1 &
  echo $! > "$PID_FILE"
  sleep 1
fi
PID=$(cat "$PID_FILE")

# Trap for helpful exit
trap 'echo; echo -e "${YELLOW}=== hostapp log tail ===${NC}"; tail -n 50 "$LOG_FILE" || true; echo; echo "To stop: kill $(cat $PID_FILE 2>/dev/null || echo $PID)"' INT TERM

# Wait for readiness
echo -e "${BLUE}INFO${NC}: Waiting up to ${WAIT_SECS}s for /healthz at $BASE_URL/healthz ..."
start_ts=$(date +%s)
while :; do
  if curl -fsS "$BASE_URL/healthz" | jq -e '.status=="ok"' >/dev/null 2>&1; then
    echo -e "${GREEN}OK${NC}: Health check passed"
    break
  fi
  now=$(date +%s)
  if [ $((now - start_ts)) -ge "$WAIT_SECS" ]; then
  die "Server did not become healthy in ${WAIT_SECS}s"
  fi
  sleep 1
done

# Briefly grep for tsnet info
TSINFO=$(grep -E "tailscale up: ip=|tsnet" "$LOG_FILE" | tail -n 1 || true)
if [ -n "$TSINFO" ]; then echo -e "${GREEN}OK${NC}: $TSINFO"; else echo -e "${YELLOW}WARN${NC}: No tsnet status found in logs yet"; fi

# Verify Talos ping
PING_URL="$BASE_URL/api/ping?addr=${TALOS_IP}:50000"
echo -e "${BLUE}INFO${NC}: Pinging Talos at $PING_URL"
PING_JSON=$(curl -fsS "$PING_URL" || true)
if [ -z "$PING_JSON" ]; then die "ping request failed"; fi
if echo "$PING_JSON" | jq -e '.ok==true' >/dev/null 2>&1; then
  RTT=$(echo "$PING_JSON" | jq -r '.rtt_ms')
  echo -e "${GREEN}OK${NC}: Talos TCP :50000 reachable (rtt_ms=$RTT)"
else
  ERRMSG=$(echo "$PING_JSON" | jq -r '.error // "unknown"')
  die "Talos ping failed: $ERRMSG"
fi

# Optional proxy check
PROXY_SNIPPET=""
if [ -n "$SVC_ADDR" ]; then
  echo -e "${BLUE}INFO${NC}: Verifying proxy to $SVC_ADDR"
  # Use curl limits; rely on app-side size caps too
  PROXY_RESP=$(curl -sS --max-time 15 -D - "$BASE_URL/proxy?to=${SVC_ADDR}&path=/" || true)
  # Normalize CRLF to LF
  PROXY_RESP=$(printf "%s" "$PROXY_RESP" | tr -d '\r')
  STATUS=$(printf "%s" "$PROXY_RESP" | awk 'NR==1{print $2}')
  HEADERS=$(printf "%s" "$PROXY_RESP" | sed -n '1,/^$/p')
  BODY=$(printf "%s" "$PROXY_RESP" | sed -n '1,/^$/d;p')
  if [ "$STATUS" = "200" ]; then
    # Check size < 10MB either via header or body length
    CL=$(printf "%s" "$HEADERS" | awk -F': ' 'tolower($1)=="content-length"{print $2}' | tail -n1)
    if [ -n "$CL" ]; then
      if [ "$CL" -gt 10485760 ] 2>/dev/null; then die "Proxy response too large ($CL bytes)"; fi
    else
      BODY_BYTES=$(printf "%s" "$BODY" | wc -c | awk '{print $1}')
      if [ "$BODY_BYTES" -gt 10485760 ] 2>/dev/null; then die "Proxy response too large ($BODY_BYTES bytes)"; fi
    fi
    PROXY_SNIPPET=$(printf "%s" "$BODY" | head -c 200 | tr '\n' ' ')
    echo -e "${GREEN}OK${NC}: Proxy returned 200 (snippet: ${PROXY_SNIPPET})"
  else
    die "Proxy to ${SVC_ADDR} failed (status=$STATUS)"
  fi
fi

# Summary
echo
echo -e "${BOLD}Summary:${NC}"
echo "  Binary       : $APP_BIN"
echo "  PID          : $PID"
echo "  Local URL    : $BASE_URL"
echo "  Config       : $CONF_FILE"
echo "  Logs         : $LOG_FILE"
if [ -n "$TSINFO" ]; then echo "  tsnet        : $TSINFO"; fi
echo "  Talos ping   : OK (rtt_ms=${RTT:-unknown})"
if [ -n "$SVC_ADDR" ]; then echo "  Proxy ${SVC_ADDR} : OK (snippet=${PROXY_SNIPPET})"; fi

exit 0
