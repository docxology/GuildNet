#!/usr/bin/env bash
# Common helpers and environment for Talos setup scripts
set -euo pipefail

REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -f "$REPO_ROOT/.env" ]; then
  # shellcheck disable=SC1090
  . "$REPO_ROOT/.env"
fi

# Tunables with defaults
PRECHECK_PORT=${PRECHECK_PORT:-50000}
PRECHECK_TIMEOUT=${PRECHECK_TIMEOUT:-3}
PRECHECK_MAX_WAIT_SECS=${PRECHECK_MAX_WAIT_SECS:-600}
PRECHECK_PING=${PRECHECK_PING:-0}
APPLY_RETRIES=${APPLY_RETRIES:-10}
APPLY_RETRY_DELAY=${APPLY_RETRY_DELAY:-5}
KUBE_READY_TRIES=${KUBE_READY_TRIES:-90}
KUBE_READY_DELAY=${KUBE_READY_DELAY:-5}
REQUIRE_ENDPOINT_MATCH_CP=${REQUIRE_ENDPOINT_MATCH_CP:-0}

CLUSTER="${CLUSTER:-mycluster}"
ENDPOINT="${ENDPOINT:-https://10.0.0.10:6443}"
CP_NODES="${CP_NODES:-10.0.0.10}"
WK_NODES="${WK_NODES:-10.0.0.20}"
CP_NODES_REAL="${CP_NODES_REAL:-}"
WK_NODES_REAL="${WK_NODES_REAL:-}"
OUT_DIR="${OUT_DIR:-./talos}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }

parse_host_port() { # input -> host port
  local in="$1"
  if [[ "$in" == *:* ]]; then
    printf '%s %s' "${in%%:*}" "${in##*:}"
  else
    printf '%s %s' "$in" "$PRECHECK_PORT"
  fi
}

real_ip_for() { # forward_host forward_port idx kind
  local fhost="$1" fport="$2" idx="$3" kind="$4" # kind=cp|wk
  local -a real_list=()
  local IFS=','
  if [[ "$kind" == "cp" && -n "$CP_NODES_REAL" ]]; then real_list=( $CP_NODES_REAL ); fi
  if [[ "$kind" == "wk" && -n "$WK_NODES_REAL" ]]; then real_list=( $WK_NODES_REAL ); fi
  if [[ ${#real_list[@]} -gt 0 ]]; then
    echo "${real_list[$idx]}"; return 0
  fi
  case "$fport" in
    50010) echo "10.0.0.10"; return 0 ;;
    50020) echo "10.0.0.20"; return 0 ;;
  esac
  echo "$fhost"
}

check_tcp() { # host port timeout_seconds
  local h="$1" p="$2" t="${3:-3}"
  if command -v nc >/dev/null 2>&1; then
    nc -z -G "$t" "$h" "$p" >/dev/null 2>&1
  else
    (exec 3<>"/dev/tcp/$h/$p") >/dev/null 2>&1 || return 1
  fi
}

# Arrays
IFS=',' read -r -a CP_ARR <<< "$CP_NODES"
if [[ -n "${WK_NODES}" ]]; then IFS=',' read -r -a WK_ARR <<< "$WK_NODES"; else WK_ARR=(); fi

first_cp_fwd="${CP_ARR[0]}"
read -r FIRST_HOST FIRST_PORT <<< "$(parse_host_port "$first_cp_fwd")"
FIRST_REAL_CP="$(real_ip_for "$FIRST_HOST" "$FIRST_PORT" 0 cp)"
