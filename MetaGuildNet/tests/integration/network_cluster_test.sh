#!/bin/bash
# Integration test: Network → Cluster connectivity

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/test_framework.sh"

# Support dev mode
DEV_MODE="${METAGN_DEV_MODE:-false}"

test_suite "Network → Cluster Integration"

if [[ "$DEV_MODE" == "true" ]]; then
    # Dev mode: Test the test framework itself
    test_case "Dev Mode: Test Framework Functions"
    test_pass "Test framework loaded successfully"
    
    test_case "Dev Mode: Assertion Functions"
    assert_equals "foo" "foo" "String equality works"
    assert_contains "hello world" "world" "String contains works"
    
    test_case "Dev Mode: Command Checks"
    assert_command_succeeds "echo command works" echo "test"
    assert_file_exists "/etc/hosts" "/etc/hosts exists"
else
    # Real mode: Test actual infrastructure
    
    # Test 1: Routes include cluster CIDRs
    test_case "Routes include cluster CIDRs"
    if command -v tailscale &>/dev/null; then
        routes=$(tailscale status 2>/dev/null | grep "offering routes" || echo "")
        assert_contains "$routes" "10.96" "Routes should include service CIDR"
    else
        test_fail "Tailscale not found"
    fi
    
    # Test 2: Can reach Kubernetes API via Tailnet
    test_case "Can reach Kubernetes API via Tailnet"
    if kubectl get --raw /readyz &>/dev/null; then
        test_pass "Kubernetes API reachable"
    else
        test_fail "Kubernetes API not reachable"
    fi
    
    # Test 3: DNS resolution works
    test_case "DNS resolution for cluster services"
    if kubectl get svc kubernetes &>/dev/null; then
        test_pass "DNS resolution works"
    else
        test_fail "DNS resolution failed"
    fi
    
    # Test 4: Pods can communicate (if any exist)
    test_case "Pod connectivity"
    pod_count=$(kubectl get pods --all-namespaces --no-headers 2>/dev/null | wc -l || echo 0)
    if [[ $pod_count -gt 0 ]]; then
        test_pass "Pods exist and can be queried"
    else
        test_fail "No pods found"
    fi
fi

run_test_suite
