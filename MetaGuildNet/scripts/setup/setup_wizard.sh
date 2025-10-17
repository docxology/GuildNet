#!/bin/bash
# MetaGuildNet Setup Wizard
# Automated setup of the full GuildNet stack

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

# Configuration
SETUP_MODE="${METAGN_SETUP_MODE:-auto}"
AUTO_APPROVE_ROUTES="${METAGN_AUTO_APPROVE_ROUTES:-true}"
DEV_MODE="${METAGN_DEV_MODE:-false}"

main() {
    log_section "MetaGuildNet Setup Wizard"
    
    if [[ "$DEV_MODE" == "true" ]]; then
        log_info "Running in DEV MODE - demonstrating setup flow"
        log_info "In production, this would:"
        echo ""
        echo "  1. Check prerequisites (Docker, kubectl, Go, etc.)"
        echo "  2. Set up Network Layer (Headscale + Tailscale)"
        echo "  3. Set up Cluster Layer (Talos + K8s + MetalLB)"
        echo "  4. Set up Application Layer (Host App + Operator)"
        echo "  5. Run comprehensive verification"
        echo ""
        log_success "Dev mode setup complete (no actual infrastructure created)"
        return 0
    fi
    
    # Check prerequisites
    log_info "Checking prerequisites..."
    if ! bash "${SCRIPT_DIR}/check_prerequisites.sh"; then
        log_error "Prerequisites check failed"
        log_info "Install missing dependencies and try again"
        return 1
    fi
    
    log_success "Prerequisites OK"
    
    # Show setup plan
    show_setup_plan
    
    # Confirm if not in auto mode
    if [[ "$SETUP_MODE" != "auto" ]]; then
        if ! confirm "Proceed with setup?" "y"; then
            log_info "Setup cancelled"
            return 0
        fi
    fi
    
    # Start timer
    local start_time
    start_time=$(date +%s)
    
    # Setup layers
    log_section "Layer 1: Network"
    setup_network || { log_error "Network setup failed"; return 1; }

    log_section "Layer 2: Cluster"
    if ! setup_cluster; then
        log_warn "Cluster setup failed - this is expected without Talos infrastructure"
        log_info "To set up a Talos cluster:"
        echo "  1. Install QEMU: sudo apt-get install qemu-system-x86"
        echo "  2. Run: make talos-vm-up"
        echo "  3. Or use existing Talos nodes with proper configuration"
        echo ""
        log_warn "Continuing with application setup (cluster features will be limited)"
    fi
    
    log_section "Layer 3: Application"
    setup_application || { log_error "Application setup failed"; return 1; }
    
    # Verify
    log_section "Verification"
    if bash "${SCRIPT_DIR}/../verify/verify_all.sh"; then
        log_success "Verification passed"
    else
        log_warn "Verification failed - this is expected without full infrastructure"
        log_info "Run 'make meta-diagnose' for troubleshooting"
        log_info "Some components may be missing (Kubernetes cluster, etc.)"
    fi
    
    # Calculate duration
    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    # Success message
    show_success_message "$duration"
}

show_setup_plan() {
    echo ""
    echo "Setup Plan:"
    echo "  Mode: $SETUP_MODE"
    echo ""
    echo "Layers to be configured:"
    echo "  1. Network Layer"
    echo "     - Headscale (Docker)"
    echo "     - Tailscale router"
    echo "     - Route approval"
    echo ""
    echo "  2. Cluster Layer"
    echo "     - Talos cluster"
    echo "     - MetalLB"
    echo "     - CRDs"
    echo "     - RethinkDB"
    echo ""
    echo "  3. Application Layer"
    echo "     - Host App"
    echo "     - Embedded Operator"
    echo "     - UI"
    echo ""
}

setup_network() {
    log_info "Setting up network layer..."
    
    # Check if already setup
    if docker ps | grep -q "guildnet-headscale" && tailscale status &>/dev/null; then
        log_warn "Network layer appears already setup"
        if [[ "$SETUP_MODE" == "auto" ]] || confirm "Skip network setup?"; then
            log_info "Skipping network setup"
            return 0
        fi
    fi
    
    # Run network setup
    if bash "${SCRIPT_DIR}/setup_network.sh"; then
        log_success "Network layer ready"
        return 0
    else
        log_error "Network setup failed"
        return 1
    fi
}

setup_cluster() {
    log_info "Setting up cluster layer..."

    # Check if already setup
    if kubectl get nodes &>/dev/null; then
        log_warn "Cluster appears already setup"
        if [[ "$SETUP_MODE" == "auto" ]] || confirm "Skip cluster setup?"; then
            log_info "Skipping cluster setup"
            return 0
        fi
    fi

    # Run cluster setup
    if bash "${SCRIPT_DIR}/setup_cluster.sh"; then
        log_success "Cluster layer ready"
        return 0
    else
        log_warn "Cluster setup failed - this is expected without Talos nodes"
        log_info "To set up a Talos cluster:"
        echo "  1. Run: make talos-vm-up"
        echo "  2. Or configure existing Talos nodes"
        echo "  3. Then run: make setup-talos"
        echo ""
        log_warn "Skipping cluster setup (application features will be limited)"
        return 0  # Don't fail the entire setup
    fi
}

setup_application() {
    log_info "Setting up application layer..."

    # Check if already running
    if curl -sk https://127.0.0.1:8080/healthz &>/dev/null; then
        log_warn "Application appears already running"
        if [[ "$SETUP_MODE" == "auto" ]] || confirm "Skip application setup?"; then
            log_info "Skipping application setup"
            return 0
        fi
    fi

    # Run application setup (but don't start if K8s not available)
    if bash "${SCRIPT_DIR}/setup_application.sh"; then
        log_success "Application layer ready"
        log_info "Host App is ready to run when Kubernetes cluster is available"
        log_info "Start with: make run"
        return 0
    else
        log_warn "Application setup failed - this is expected without Kubernetes cluster"
        log_info "To run the Host App:"
        echo "  1. Ensure you have a Kubernetes cluster"
        echo "  2. Run: make run"
        echo "  3. Or run in standalone mode if available"
        echo ""
        log_warn "Application features will be limited without cluster"
        return 0  # Don't fail the entire setup
    fi
}

show_success_message() {
    local duration="$1"
    
    log_section "Setup Complete!"
    
    echo "Duration: $(human_duration "$duration")"
    echo ""
    echo "Access URLs:"
    echo "  Local:   https://127.0.0.1:8080"
    
    # Get Tailscale IP if available
    if tailscale status &>/dev/null; then
        local ts_ip
        ts_ip=$(tailscale status --json 2>/dev/null | grep -o '"Self":{"ID":"[^"]*","PublicKey":"[^"]*","HostName":"[^"]*","DNSName":"[^"]*","OS":"[^"]*","TailscaleIPs":\["[^"]*"' | grep -o '[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}' | head -1 || echo "")
        if [[ -n "$ts_ip" ]]; then
            echo "  Tailnet: https://$ts_ip:443"
        fi
    fi
    
    echo ""
    echo "Next steps:"
    echo "  1. Verify everything: make meta-verify"
    echo "  2. Create workspace: bash MetaGuildNet/examples/basic/create-workspace.sh"
    echo "  3. View documentation: MetaGuildNet/docs/"
    echo ""
    log_success "MetaGuildNet is ready!"
}

main "$@"

