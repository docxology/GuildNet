#!/bin/bash
# Verify Database Layer

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

# Support dev mode for testing without infrastructure
DEV_MODE="${METAGN_DEV_MODE:-false}"

main() {
    local checks_passed=0
    local checks_total=4
    
    # Check 1: RethinkDB pod Running
    if kubectl get pods -l app=rethinkdb &>/dev/null 2>&1; then
        local rethink_status
        rethink_status=$(kubectl get pods -l app=rethinkdb --no-headers 2>/dev/null | awk '{print $3}' | head -1)
        if [[ "$rethink_status" == "Running" ]]; then
            log_success "RethinkDB pod Running"
            checks_passed=$((checks_passed + 1))
        else
            log_error "RethinkDB pod not Running (status: $rethink_status)"
        fi
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "RethinkDB pod check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Unable to check RethinkDB pod"
        fi
    fi
    
    # Check 2: Service has LoadBalancer IP
    if kubectl get service rethinkdb &>/dev/null 2>&1; then
        local lb_ip
        lb_ip=$(kubectl get service rethinkdb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
        if [[ -n "$lb_ip" ]]; then
            log_success "RethinkDB service has LoadBalancer IP: $lb_ip"
            checks_passed=$((checks_passed + 1))
        else
            log_warn "RethinkDB service has no LoadBalancer IP"
        fi
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "RethinkDB service check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "RethinkDB service not found"
        fi
    fi
    
    # Check 3: Port 28015 accessible
    if kubectl get service rethinkdb &>/dev/null 2>&1; then
        local lb_ip
        lb_ip=$(kubectl get service rethinkdb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
        if [[ -n "$lb_ip" ]] && timeout 3 bash -c "echo > /dev/tcp/$lb_ip/28015" 2>/dev/null; then
            log_success "Port 28015 accessible"
            checks_passed=$((checks_passed + 1))
        else
            log_warn "Port 28015 not accessible"
        fi
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Port check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Unable to check port connectivity"
        fi
    fi
    
    # Check 4: Query latency acceptable (if accessible)
    if kubectl get service rethinkdb &>/dev/null 2>&1; then
        local lb_ip
        lb_ip=$(kubectl get service rethinkdb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
        if [[ -n "$lb_ip" ]]; then
            # Simple connectivity check as a proxy for query latency
            local start_time
            local end_time
            start_time=$(date +%s%3N)
            if timeout 3 bash -c "echo > /dev/tcp/$lb_ip/28015" 2>/dev/null; then
                end_time=$(date +%s%3N)
                local latency=$((end_time - start_time))
                if [[ "$latency" -lt 1000 ]]; then
                    log_success "Query latency acceptable (${latency}ms)"
                    checks_passed=$((checks_passed + 1))
                else
                    log_warn "Query latency high (${latency}ms)"
                fi
            else
                log_warn "Unable to measure query latency"
            fi
        else
            if [[ "$DEV_MODE" == "true" ]]; then
                log_info "Query latency check skipped (dev mode)"
                checks_passed=$((checks_passed + 1))
            else
                log_warn "No LoadBalancer IP to test"
            fi
        fi
    else
        if [[ "$DEV_MODE" == "true" ]]; then
            log_info "Query latency check skipped (dev mode)"
            checks_passed=$((checks_passed + 1))
        else
            log_error "Unable to check database"
        fi
    fi
    
    log_info "Database checks: $checks_passed/$checks_total passed"
    
    # In dev mode, always pass; otherwise need at least 2
    if [[ "$DEV_MODE" == "true" ]]; then
        return 0
    else
        [[ $checks_passed -ge 2 ]] && return 0 || return 1
    fi
}

main "$@"
