#!/bin/bash
# E2E test: Workspace lifecycle

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/test_framework.sh"

# Support dev mode
DEV_MODE="${METAGN_DEV_MODE:-false}"

test_suite "Workspace Lifecycle E2E"

if [[ "$DEV_MODE" == "true" ]]; then
    # Dev mode: Test workspace YAML and CRD structure
    test_case "Dev Mode: CRD Files Exist"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
    assert_file_exists "$PROJECT_ROOT/config/crd/guildnet.io_workspaces.yaml" "Workspace CRD exists"
    assert_file_exists "$PROJECT_ROOT/config/crd/guildnet.io_capabilities.yaml" "Capability CRD exists"
    
    test_case "Dev Mode: Example Workspace YAMLs Exist"
    assert_file_exists "$PROJECT_ROOT/k8s/agent-example.yaml" "Agent example exists"
    
    test_case "Dev Mode: Workspace API Types"
    if [[ -f "$PROJECT_ROOT/api/v1alpha1/types.go" ]]; then
        test_pass "Workspace API types defined"
    else
        test_fail "Workspace API types not found"
    fi
    
    test_case "Dev Mode: Operator Controller"
    if [[ -f "$PROJECT_ROOT/internal/operator/workspace_controller.go" ]]; then
        test_pass "Workspace controller exists"
    else
        test_fail "Workspace controller not found"
    fi
else
    # Real mode: Test actual workspace creation and management
    
    test_case "CRDs are installed"
    if kubectl get crd workspaces.guildnet.io &>/dev/null; then
        test_pass "Workspace CRD installed"
    else
        test_fail "Workspace CRD not installed"
    fi
    
    test_case "Create workspace"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
    workspace_yaml=$(mktemp)
    cat > "$workspace_yaml" << 'EOF'
apiVersion: guildnet.io/v1alpha1
kind: Workspace
metadata:
  name: test-workspace
spec:
  image: alpine:latest
  command: ["sleep", "infinity"]
EOF
    
    if kubectl apply -f "$workspace_yaml" &>/dev/null; then
        test_pass "Workspace created"
    else
        test_fail "Failed to create workspace"
    fi
    
    test_case "Workspace reconciled"
    sleep 5
    if kubectl get workspace test-workspace &>/dev/null; then
        test_pass "Workspace exists in cluster"
    else
        test_fail "Workspace not found"
    fi
    
    test_case "Cleanup workspace"
    if kubectl delete -f "$workspace_yaml" &>/dev/null; then
        test_pass "Workspace deleted"
    else
        test_fail "Failed to delete workspace"
    fi
    
    rm -f "$workspace_yaml"
fi

run_test_suite
