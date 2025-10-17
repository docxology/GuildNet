#!/bin/bash
# Setup Network Layer (Headscale + Tailscale)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

PROJECT_ROOT="$(get_project_root)"
AUTO_APPROVE="${METAGN_AUTO_APPROVE_ROUTES:-true}"

main() {
    log_subsection "Network Layer Setup"
    
    # Setup Headscale
    setup_headscale
    
    # Setup Tailscale router
    setup_tailscale
    
    # Approve routes
    if [[ "$AUTO_APPROVE" == "true" ]]; then
        approve_routes
    fi
    
    # Verify
    verify_network
    
    log_success "Network layer setup complete"
}

setup_headscale() {
    log_info "Setting up Headscale..."
    
    cd "$PROJECT_ROOT" || return 1
    
    # Start Headscale
    if ! make headscale-up; then
        log_error "Failed to start Headscale"
        return 1
    fi
    
    # Sync LAN IP first (creates .env if needed and sets server URL)
    if [[ "${SYNC_LAN_IP:-true}" == "true" ]]; then
        make env-sync-lan || true
    fi

    # Bootstrap (now .env exists with server URL)
    if ! make headscale-bootstrap; then
        log_error "Failed to bootstrap Headscale"
        return 1
    fi
    
    log_success "Headscale running"
}

setup_tailscale() {
    log_info "Setting up Tailscale router..."
    
    cd "$PROJECT_ROOT" || return 1
    
    # Install if needed
    if ! command -v tailscale &>/dev/null; then
        log_info "Installing Tailscale..."
        if ! make router-install; then
            log_error "Failed to install Tailscale"
            return 1
        fi
    fi
    
    # Ensure daemon running
    make router-daemon || make router-daemon-sudo || true
    
    # Grant operator privilege
    make router-grant-operator || make router-grant-operator-sudo || true
    
    # Bring up router
    if ! make router-up; then
        log_error "Failed to bring up Tailscale router"
        return 1
    fi
    
    log_success "Tailscale router running"
}

approve_routes() {
    log_info "Approving routes..."
    
    cd "$PROJECT_ROOT" || return 1
    
    # Wait a bit for routes to be advertised
    sleep 5
    
    # Approve routes
    if ! make headscale-approve-routes; then
        log_warn "Failed to auto-approve routes"
        log_info "You may need to manually approve routes"
        return 0
    fi
    
    log_success "Routes approved"
}

verify_network() {
    log_info "Verifying network layer..."
    
    # Check Headscale
    if ! docker ps | grep -q "guildnet-headscale"; then
        log_error "Headscale not running"
        return 1
    fi
    
    # Check Tailscale
    if ! tailscale status &>/dev/null; then
        log_error "Tailscale not connected"
        return 1
    fi
    
    # Check routes
    if ! tailscale status | grep -q "offering routes"; then
        log_warn "Routes may not be advertised"
    fi
    
    log_success "Network layer verified"
}

main "$@"

