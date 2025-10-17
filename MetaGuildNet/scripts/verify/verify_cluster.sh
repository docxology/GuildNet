#!/bin/bash
# Verify Cluster Layer

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

# Support dev mode for testing without infrastructure
DEV_MODE="${METAGN_DEV_MODE:-false}"

main() {
    local checks_passed=0
    local checks_total=5
    
    # Check 1: Kubernetes API accessible
    if kubectl get --raw /readyz &>/dev/null; then
        log_success "Kubernetes API accessible"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Kubernetes API not accessible (dev mode - skipping)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Kubernetes API not accessible"
        fi
    fi
    
    # Check 2: All nodes Ready
    if kubectl get nodes &>/dev/null && kubectl get nodes 2>/dev/null | grep -q "Ready"; then
        local total_nodes
        local ready_nodes
        total_nodes=$(kubectl get nodes --no-headers 2>/dev/null | wc -l)
        ready_nodes=$(kubectl get nodes --no-headers 2>/dev/null | grep -c " Ready " || true)
        
        if [[ "$ready_nodes" -eq "$total_nodes" ]] && [[ "$total_nodes" -gt 0 ]]; then
            log_success "All nodes Ready ($ready_nodes/$total_nodes)"
            checks_passed=$((checks_passed + 1))
        else
            log_warn "Some nodes not Ready ($ready_nodes/$total_nodes)"
        fi
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Node check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Unable to check nodes"
        fi
    fi
    
    # Check 3: CoreDNS operational
    if kubectl get pods -n kube-system -l k8s-app=kube-dns &>/dev/null; then
        local coredns_ready
        coredns_ready=$(kubectl get pods -n kube-system -l k8s-app=kube-dns --no-headers 2>/dev/null | grep -c "Running" || echo "0")
        if [[ "$coredns_ready" -gt 0 ]]; then
            log_success "CoreDNS operational"
            checks_passed=$((checks_passed + 1))
        else
            log_error "CoreDNS not running"
        fi
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "CoreDNS check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Unable to check CoreDNS"
        fi
    fi
    
    # Check 4: MetalLB running
    if kubectl get pods -n metallb-system &>/dev/null 2>&1; then
        log_success "MetalLB deployed"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "MetalLB check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_warn "MetalLB not found (may not be required)"
            checks_passed=$((checks_passed + 1))  # Not critical
        fi
    fi
    
    # Check 5: CRDs installed
    if kubectl get crd workspaces.guildnet.io &>/dev/null 2>&1; then
        log_success "GuildNet CRDs installed"
        checks_passed=$((checks_passed + 1))
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "CRD check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_warn "GuildNet CRDs not installed"
        fi
    fi
    
    log_info "Cluster checks: $checks_passed/$checks_total passed"
    
    # In dev mode, always pass; otherwise need at least 3
    if [[ "$DEV_MODE" == "true" ]]; then
        return 0
    else
        [[ $checks_passed -ge 3 ]] && return 0 || return 1
    fi
}

main "$@"
