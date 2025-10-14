#!/bin/bash
# Setup Cluster Layer (Talos + K8s + Add-ons)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

PROJECT_ROOT="$(get_project_root)"

main() {
    log_subsection "Cluster Layer Setup"
    
    # Setup Talos cluster
    setup_talos
    
    # Deploy add-ons
    deploy_addons
    
    # Verify
    verify_cluster
    
    log_success "Cluster layer setup complete"
}

setup_talos() {
    log_info "Setting up Talos cluster..."
    
    cd "$PROJECT_ROOT" || return 1
    
    # Run Talos setup
    if ! make setup-talos; then
        log_error "Failed to setup Talos cluster"
        return 1
    fi
    
    # Wait for API (skip if no kubeconfig)
    if [[ -n "${KUBECONFIG:-}" ]] && [[ -f "${KUBECONFIG:-}" ]]; then
        log_info "Waiting for Kubernetes API..."
        if ! wait_for "Kubernetes API" 180 kubectl get --raw /readyz; then
            log_error "Kubernetes API not ready"
            return 1
        fi
    else
        log_warn "No kubeconfig available, skipping Kubernetes API wait"
        log_info "To wait for Kubernetes API when cluster is ready:"
        echo "  Run: kubectl cluster-info"
        return 0  # Don't fail
    fi
    
    log_success "Talos cluster running"
}

deploy_addons() {
    log_info "Deploying Kubernetes add-ons..."

    cd "$PROJECT_ROOT" || return 1

    # Check if Kubernetes is available
    if ! kubectl cluster-info &>/dev/null; then
        log_warn "Kubernetes cluster not available, skipping add-on deployment"
        log_info "To deploy add-ons when cluster is ready:"
        echo "  Run: make deploy-k8s-addons"
        return 0  # Don't fail
    fi

    # Deploy all add-ons (MetalLB, CRDs, RethinkDB, etc.)
    if ! make deploy-k8s-addons; then
        log_error "Failed to deploy add-ons"
        return 1
    fi

    # Wait for critical pods
    log_info "Waiting for add-ons to be ready..."

    # Wait for MetalLB
    wait_for "MetalLB controller" 120 kubectl get pods -n metallb-system -l app.kubernetes.io/component=controller --no-headers | grep -q Running || true

    # Wait for RethinkDB
    wait_for "RethinkDB" 180 kubectl get pods -l app=rethinkdb --no-headers | grep -q Running || true

    log_success "Add-ons deployed"
}

verify_cluster() {
    log_info "Verifying cluster layer..."

    # Check if Kubernetes is available
    if ! kubectl cluster-info &>/dev/null; then
        log_warn "Kubernetes cluster not available, skipping cluster verification"
        log_info "To verify cluster when ready:"
        echo "  Run: kubectl get nodes"
        return 0  # Don't fail
    fi

    # Check nodes
    if ! kubectl get nodes | grep -q Ready; then
        log_error "No nodes ready"
        return 1
    fi

    # Check system pods
    if ! kubectl get pods -n kube-system | grep -q coredns; then
        log_error "CoreDNS not running"
        return 1
    fi

    # Check MetalLB
    if ! kubectl get pods -n metallb-system &>/dev/null; then
        log_warn "MetalLB not found"
    fi

    # Check CRDs
    if ! kubectl get crds | grep -q guildnet.io; then
        log_warn "GuildNet CRDs not found"
    fi

    log_success "Cluster layer verified"
}

main "$@"

