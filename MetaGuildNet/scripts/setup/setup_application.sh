#!/bin/bash
# Setup Application Layer (Host App + Operator)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

PROJECT_ROOT="$(get_project_root)"

main() {
    log_subsection "Application Layer Setup"
    
    # Build application
    build_application
    
    # Start application (background)
    start_application
    
    # Verify
    verify_application
    
    log_success "Application layer setup complete"
}

build_application() {
    log_info "Building application..."
    
    cd "$PROJECT_ROOT" || return 1
    
    # Build backend
    if ! make build-backend; then
        log_error "Failed to build backend"
        return 1
    fi
    
    # Build UI
    if ! make build-ui; then
        log_error "Failed to build UI"
        return 1
    fi
    
    log_success "Application built"
}

start_application() {
    log_info "Starting Host App..."
    
    cd "$PROJECT_ROOT" || return 1
    
    # Note: In production, this would be run via systemd or similar
    # For setup wizard, we just verify it can start, then inform user
    
    log_info "Application ready to run"
    log_info "Start with: make run"
}

verify_application() {
    log_info "Verifying application can start..."
    
    cd "$PROJECT_ROOT" || return 1
    
    # Check binaries exist
    if [[ ! -f "$PROJECT_ROOT/bin/hostapp" ]]; then
        log_error "Host app binary not found"
        return 1
    fi
    
    if [[ ! -d "$PROJECT_ROOT/ui/dist" ]]; then
        log_error "UI dist not found"
        return 1
    fi
    
    # Check certificates
    if [[ ! -f "$PROJECT_ROOT/certs/server.crt" ]] && [[ ! -f "$PROJECT_ROOT/certs/dev.crt" ]]; then
        log_warn "No TLS certificates found, will use auto-generated"
    fi
    
    log_success "Application verified"
}

main "$@"

