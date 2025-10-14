#!/bin/bash
# Quick health check of all layers

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

main() {
    log_section "Quick Health Check"
    
    local healthy_count=0
    local total_count=4
    
    # Network
    if check_network; then
        log_success "Network: Healthy"
        ((healthy_count++))
    else
        log_error "Network: Unhealthy"
    fi
    
    # Cluster
    if check_cluster; then
        log_success "Cluster: Healthy"
        ((healthy_count++))
    else
        log_error "Cluster: Unhealthy"
    fi
    
    # Database
    if check_database; then
        log_success "Database: Healthy"
        ((healthy_count++))
    else
        log_warn "Database: Unhealthy"
    fi
    
    # Application
    if check_application; then
        log_success "Application: Healthy"
        ((healthy_count++))
    else
        log_warn "Application: Unhealthy (may not be started)"
    fi
    
    echo ""
    log_info "Health: $healthy_count/$total_count components healthy"
    
    if [[ $healthy_count -ge 3 ]]; then
        log_success "Overall system health: Good"
        return 0
    else
        log_error "Overall system health: Degraded"
        log_info "Run 'make meta-verify' for detailed diagnostics"
        return 1
    fi
}

check_network() {
    tailscale status &>/dev/null
}

check_cluster() {
    kubectl get --raw /readyz &>/dev/null
}

check_database() {
    kubectl get pods -l app=rethinkdb 2>/dev/null | grep -q Running
}

check_application() {
    curl -sk https://127.0.0.1:8080/healthz 2>/dev/null | grep -q ok
}

main "$@"

