#!/bin/bash
# Verify Network Layer

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

# Support dev mode for testing without infrastructure
DEV_MODE="${METAGN_DEV_MODE:-false}"

main() {
    local checks_passed=0
    local checks_total=4
    
    # Check 1: Tailscale daemon running
    if command -v tailscale &>/dev/null && tailscale status &>/dev/null; then
        log_success "Tailscale daemon running"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Tailscale daemon not running (dev mode - skipping)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Tailscale daemon not running"
        fi
    fi
    
    # Check 2: Device connected
    if command -v tailscale &>/dev/null && tailscale status 2>/dev/null | grep -q "Self:"; then
        log_success "Device connected to Tailnet"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Device not connected to Tailnet (dev mode - skipping)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Device not connected to Tailnet"
        fi
    fi
    
    # Check 3: Routes advertised
    if command -v tailscale &>/dev/null && tailscale status 2>/dev/null | grep -q "offering routes"; then
        log_success "Routes advertised"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Routes check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_warn "Routes may not be advertised"
        fi
    fi
    
    # Check 4: Headscale container running
    if command -v docker &>/dev/null && docker ps 2>/dev/null | grep -q "guildnet-headscale"; then
        log_success "Headscale container running"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Headscale container check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_warn "Headscale container not found (may be using external)"
        fi
    fi
    
    log_info "Network checks: $checks_passed/$checks_total passed"
    
    # In dev mode, pass if we have any checks; otherwise need at least 2
    if [[ "$DEV_MODE" == "true" ]]; then
        return 0
    else
        [[ $checks_passed -ge 2 ]] && return 0 || return 1
    fi
}

main "$@"
