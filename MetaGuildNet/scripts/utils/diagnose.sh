#!/bin/bash
# Diagnose issues across all layers

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

main() {
    log_section "MetaGuildNet Diagnostics"
    
    diagnose_network
    diagnose_cluster
    diagnose_database
    diagnose_application
    
    echo ""
    log_info "For detailed verification, run: make meta-verify"
    log_info "For diagnostic bundle, run: make -C MetaGuildNet export-diagnostics"
}

diagnose_network() {
    log_subsection "Network Layer"
    
    # Tailscale
    if command -v tailscale &>/dev/null; then
        if tailscale status &>/dev/null; then
            log_success "Tailscale connected"
            tailscale status | head -5
        else
            log_error "Tailscale not connected"
            log_info "→ Check: systemctl status tailscaled"
            log_info "→ Start: make router-up"
        fi
    else
        log_error "Tailscale not installed"
        log_info "→ Install: make router-install"
    fi
    
    echo ""
    
    # Headscale
    if docker ps 2>/dev/null | grep -q "guildnet-headscale"; then
        log_success "Headscale running"
    else
        log_warn "Headscale not running (may be using external)"
        log_info "→ Start: make headscale-up"
    fi
}

diagnose_cluster() {
    log_subsection "Cluster Layer"
    
    # Kubernetes
    if kubectl version --client &>/dev/null; then
        if kubectl get --raw /readyz &>/dev/null; then
            log_success "Kubernetes API accessible"
            kubectl get nodes -o wide 2>/dev/null || true
        else
            log_error "Kubernetes API not accessible"
            log_info "→ Check: kubectl cluster-info"
            log_info "→ Setup: make setup-talos"
        fi
    else
        log_error "kubectl not found"
        log_info "→ Install kubectl"
    fi
}

diagnose_database() {
    log_subsection "Database Layer"
    
    if kubectl get pods -l app=rethinkdb 2>/dev/null | grep -q Running; then
        log_success "RethinkDB running"
        kubectl get pods -l app=rethinkdb -o wide
    else
        log_warn "RethinkDB not running"
        log_info "→ Deploy: make deploy-k8s-addons"
    fi
}

diagnose_application() {
    log_subsection "Application Layer"
    
    if curl -sk https://127.0.0.1:8080/healthz 2>/dev/null | grep -q ok; then
        log_success "Host App running"
        curl -sk https://127.0.0.1:8080/api/ui-config 2>/dev/null | head -5 || true
    else
        log_warn "Host App not running"
        log_info "→ Start: make run"
    fi
}

main "$@"

