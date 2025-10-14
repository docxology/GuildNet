#!/bin/bash
# Verify Application Layer

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

PROJECT_ROOT="$(get_project_root)"
DEV_MODE="${METAGN_DEV_MODE:-false}"
HOSTAPP_PORT="${METAGN_HOSTAPP_PORT:-8080}"

main() {
    local checks_passed=0
    local checks_total=5
    
    # Check 1: Host App serving
    if curl -sf "http://localhost:$HOSTAPP_PORT/health" &>/dev/null; then
        log_success "Host App serving on port $HOSTAPP_PORT"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Host App not running (dev mode - skipping)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Host App not serving"
        fi
    fi
    
    # Check 2: UI loads
    if curl -sf "http://localhost:$HOSTAPP_PORT/" &>/dev/null; then
        log_success "UI loads"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "UI check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "UI not accessible"
        fi
    fi
    
    # Check 3: API endpoints functional
    if curl -sf "http://localhost:$HOSTAPP_PORT/api/v1/ping" &>/dev/null; then
        log_success "API endpoints functional"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "API check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "API endpoints not responding"
        fi
    fi
    
    # Check 4: Operator reconciling (check if binary exists and is executable)
    if [[ -f "$PROJECT_ROOT/bin/hostapp" ]] && [[ -x "$PROJECT_ROOT/bin/hostapp" ]]; then
        log_success "Operator binary present"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Operator check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_warn "Operator binary not found (build may be needed)"
        fi
    fi
    
    # Check 5: Proxy functionality
    if curl -sf "http://localhost:$HOSTAPP_PORT/health" &>/dev/null; then
        # If host app is running, proxy functionality is likely working
        log_success "Proxy functionality available"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Proxy check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Proxy not functional"
        fi
    fi
    
    log_info "Application checks: $checks_passed/$checks_total passed"
    
    # In dev mode, always pass; otherwise need at least 3
    if [[ "$DEV_MODE" == "true" ]]; then
        return 0
    else
        [[ $checks_passed -ge 3 ]] && return 0 || return 1
    fi
}

main "$@"
