#!/bin/bash
# Common utility functions for MetaGuildNet scripts
# Source this file in all scripts: source "$(dirname "$0")/../lib/common.sh"

# Prevent multiple sourcing
[[ -n "${_METAGUILDNET_COMMON_LOADED:-}" ]] && return 0
readonly _METAGUILDNET_COMMON_LOADED=1

# Colors
readonly COLOR_RESET='\033[0m'
readonly COLOR_RED='\033[0;31m'
readonly COLOR_GREEN='\033[0;32m'
readonly COLOR_YELLOW='\033[1;33m'
readonly COLOR_BLUE='\033[0;34m'
readonly COLOR_MAGENTA='\033[0;35m'
readonly COLOR_CYAN='\033[0;36m'
readonly COLOR_BOLD='\033[1m'

# Log level
LOG_LEVEL="${METAGN_LOG_LEVEL:-info}"

# Log functions
log_debug() {
    [[ "$LOG_LEVEL" == "debug" ]] || return 0
    echo -e "${COLOR_CYAN}[DEBUG]${COLOR_RESET} $*" >&2
}

log_info() {
    echo -e "${COLOR_BLUE}[INFO]${COLOR_RESET} $*" >&2
}

log_success() {
    echo -e "${COLOR_GREEN}[✓]${COLOR_RESET} $*" >&2
}

log_warn() {
    echo -e "${COLOR_YELLOW}[⚠]${COLOR_RESET} $*" >&2
}

log_error() {
    echo -e "${COLOR_RED}[✗]${COLOR_RESET} $*" >&2
}

log_section() {
    echo ""
    echo -e "${COLOR_BOLD}${COLOR_MAGENTA}╔════════════════════════════════════════╗${COLOR_RESET}"
    printf "${COLOR_BOLD}${COLOR_MAGENTA}║${COLOR_RESET} %-38s ${COLOR_BOLD}${COLOR_MAGENTA}║${COLOR_RESET}\n" "$*"
    echo -e "${COLOR_BOLD}${COLOR_MAGENTA}╚════════════════════════════════════════╝${COLOR_RESET}"
    echo ""
}

log_subsection() {
    echo ""
    echo -e "${COLOR_BOLD}[$*]${COLOR_RESET}"
}

# Confirmation prompt
confirm() {
    local prompt="${1:-Continue?}"
    local default="${2:-n}"
    
    if [[ "$default" == "y" ]]; then
        read -r -p "$prompt [Y/n]: " response
        response="${response:-y}"
    else
        read -r -p "$prompt [y/N]: " response
        response="${response:-n}"
    fi
    
    case "$response" in
        [yY][eE][sS]|[yY]) return 0 ;;
        *) return 1 ;;
    esac
}

# Require command
require_command() {
    local cmd="$1"
    local install_msg="${2:-Install $cmd}"
    
    if ! command -v "$cmd" &> /dev/null; then
        log_error "$cmd not found"
        log_info "$install_msg"
        return 1
    fi
    
    log_debug "$cmd found: $(command -v "$cmd")"
    return 0
}

# Check if running as root
is_root() {
    [[ $EUID -eq 0 ]]
}

# Require root
require_root() {
    if ! is_root; then
        log_error "This script must be run as root"
        return 1
    fi
}

# Get OS type
get_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "macos" ;;
        *)       echo "unknown" ;;
    esac
}

# Get distribution (Linux only)
get_distro() {
    if [[ -f /etc/os-release ]]; then
        # shellcheck disable=SC1091
        source /etc/os-release
        echo "$ID"
    else
        echo "unknown"
    fi
}

# Retry function with exponential backoff
retry() {
    local max_attempts="${1}"
    local delay="${2}"
    local max_delay="${3:-300}"
    local attempt=1
    shift 3
    local cmd=("$@")
    
    while (( attempt <= max_attempts )); do
        log_debug "Attempt $attempt/$max_attempts: ${cmd[*]}"
        
        if "${cmd[@]}"; then
            return 0
        fi
        
        if (( attempt < max_attempts )); then
            log_warn "Command failed, retrying in ${delay}s..."
            sleep "$delay"
            delay=$(( delay * 2 ))
            (( delay > max_delay )) && delay=$max_delay
        fi
        
        (( attempt++ ))
    done
    
    log_error "Command failed after $max_attempts attempts"
    return 1
}

# Wait for condition with timeout
wait_for() {
    local description="$1"
    local timeout="$2"
    shift 2
    local cmd=("$@")
    
    local elapsed=0
    local interval=2
    
    log_info "Waiting for: $description (timeout: ${timeout}s)"
    
    while (( elapsed < timeout )); do
        if "${cmd[@]}" &> /dev/null; then
            log_success "$description ready (${elapsed}s)"
            return 0
        fi
        
        sleep "$interval"
        (( elapsed += interval ))
        
        # Progress indicator
        if (( elapsed % 10 == 0 )); then
            log_debug "Still waiting... (${elapsed}s / ${timeout}s)"
        fi
    done
    
    log_error "$description not ready after ${timeout}s"
    return 1
}

# Load environment file
load_env() {
    local env_file="${1:-.env}"
    
    if [[ ! -f "$env_file" ]]; then
        log_debug "Environment file not found: $env_file"
        return 1
    fi
    
    log_debug "Loading environment from: $env_file"
    
    # Export variables (ignoring comments and empty lines)
    while IFS='=' read -r key value; do
        # Skip comments and empty lines
        [[ "$key" =~ ^#.*$ ]] && continue
        [[ -z "$key" ]] && continue
        
        # Remove quotes from value
        value="${value%\"}"
        value="${value#\"}"
        value="${value%\'}"
        value="${value#\'}"
        
        # Export
        export "$key=$value"
        log_debug "Loaded: $key=$value"
    done < <(grep -v '^\s*#' "$env_file" | grep -v '^\s*$')
}

# Get project root
get_project_root() {
    local dir="$PWD"
    while [[ "$dir" != "/" ]]; do
        if [[ -f "$dir/go.mod" ]] && [[ -d "$dir/MetaGuildNet" ]]; then
            echo "$dir"
            return 0
        fi
        dir="$(dirname "$dir")"
    done
    
    log_error "Could not find project root"
    return 1
}

# Ensure directory exists
ensure_dir() {
    local dir="$1"
    if [[ ! -d "$dir" ]]; then
        log_debug "Creating directory: $dir"
        mkdir -p "$dir"
    fi
}

# Cleanup trap (available for scripts to use, but not set by default)
cleanup_on_exit() {
    local exit_code=$?
    log_debug "Cleanup on exit (code: $exit_code)"
    # Perform any cleanup operations here if needed
    # Note: Don't call exit here to avoid infinite loops
}

# Scripts can set this trap if needed:
# trap cleanup_on_exit EXIT

# Timestamp
timestamp() {
    date +"%Y-%m-%d %H:%M:%S"
}

timestamp_iso() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
}

# Duration in human readable format
human_duration() {
    local seconds="$1"
    
    if (( seconds < 60 )); then
        echo "${seconds}s"
    elif (( seconds < 3600 )); then
        printf "%dm %ds\n" $((seconds / 60)) $((seconds % 60))
    else
        printf "%dh %dm %ds\n" $((seconds / 3600)) $(( (seconds % 3600) / 60 )) $((seconds % 60))
    fi
}

# JSON escape
json_escape() {
    local string="$1"
    string="${string//\\/\\\\}"
    string="${string//\"/\\\"}"
    string="${string//$'\n'/\\n}"
    string="${string//$'\r'/\\r}"
    string="${string//$'\t'/\\t}"
    echo "$string"
}

# Check if command succeeded
check_status() {
    local status=$?
    local description="$1"
    
    if (( status == 0 )); then
        log_success "$description"
        return 0
    else
        log_error "$description (exit code: $status)"
        return $status
    fi
}

# Progress bar
progress_bar() {
    local current="$1"
    local total="$2"
    local width=50
    
    local percent=$(( current * 100 / total ))
    local filled=$(( current * width / total ))
    local empty=$(( width - filled ))
    
    printf "\r["
    printf "%${filled}s" | tr ' ' '='
    printf "%${empty}s" | tr ' ' ' '
    printf "] %3d%%" "$percent"
}

# Spinner
spinner() {
    local pid=$1
    local message="${2:-Working...}"
    local delay=0.1
    local spinstr='|/-\'
    
    while ps -p "$pid" > /dev/null 2>&1; do
        local temp=${spinstr#?}
        printf "\r%s [%c]  " "$message" "$spinstr"
        spinstr=$temp${spinstr%"$temp"}
        sleep "$delay"
    done
    
    printf "\r%s [Done]\n" "$message"
}

# Initialize common settings
init_common() {
    # Set strict mode
    set -euo pipefail
    
    # Get project root
    PROJECT_ROOT="$(get_project_root)" || return 1
    export PROJECT_ROOT
    
    # Load environment if exists
    if [[ -f "$PROJECT_ROOT/.env" ]]; then
        # shellcheck disable=SC1091
        load_env "$PROJECT_ROOT/.env" || true
    fi
    
    log_debug "Common library initialized"
    log_debug "Project root: $PROJECT_ROOT"
    log_debug "Log level: $LOG_LEVEL"
}

# Version check
version_compare() {
    local version1="$1"
    local version2="$2"
    
    if [[ "$version1" == "$version2" ]]; then
        return 0
    fi
    
    local IFS=.
    local i ver1=($version1) ver2=($version2)
    
    for ((i=0; i<${#ver1[@]}; i++)); do
        if (( ${ver1[i]} > ${ver2[i]:-0} )); then
            return 1
        elif (( ${ver1[i]} < ${ver2[i]:-0} )); then
            return 2
        fi
    done
    
    return 0
}

# Export functions for use in subshells
export -f log_debug log_info log_success log_warn log_error
export -f log_section log_subsection
export -f confirm require_command is_root require_root
export -f get_os get_distro retry wait_for
export -f ensure_dir timestamp timestamp_iso human_duration
export -f json_escape check_status

log_debug "Common library loaded"

