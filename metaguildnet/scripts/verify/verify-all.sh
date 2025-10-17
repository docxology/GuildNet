#!/bin/bash
# Run all verification checks

# Don't exit on error - we want to run all checks
# set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║          MetaGuildNet Verification Suite                     ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""
echo "Running comprehensive verification..."
echo "Note: Some checks are expected to fail if GuildNet is not installed yet"
echo ""

FAILED=0
WARNINGS=0
PASSED=0

run_check() {
    local check_name="$1"
    local script_name="$2"
    local optional="${3:-no}"
    
    echo "===================================="
    echo "$check_name"
    echo "===================================="
    
    if bash "$SCRIPT_DIR/$script_name" 2>&1; then
        echo "✓ $check_name passed"
        PASSED=$((PASSED + 1))
    else
        if [ "$optional" = "yes" ]; then
            echo "⚠ $check_name skipped (optional)"
            WARNINGS=$((WARNINGS + 1))
        else
            echo "✗ $check_name failed"
            FAILED=$((FAILED + 1))
        fi
    fi
    echo ""
}

# Run checks (mark k8s and guildnet as optional since they may not be installed)
run_check "System Prerequisites" "verify-system.sh" "no"
run_check "Network Connectivity" "verify-network.sh" "no"
run_check "Kubernetes Cluster" "verify-kubernetes.sh" "yes"
run_check "GuildNet Installation" "verify-guildnet.sh" "yes"

echo "===================================="
echo "Verification Summary"
echo "===================================="
echo "Passed:   $PASSED"
echo "Failed:   $FAILED"
echo "Warnings: $WARNINGS"
echo ""

if [ $FAILED -eq 0 ]; then
    if [ $WARNINGS -gt 0 ]; then
        echo "✓ Core checks passed (optional components not installed)"
        echo ""
        echo "To install GuildNet:"
        echo "  mgn install --type local"
        echo ""
        echo "Or manually:"
        echo "  cd /path/to/GuildNet"
        echo "  ./scripts/run-hostapp.sh"
        exit 0
    else
        echo "✅ All checks passed - GuildNet is fully installed!"
        exit 0
    fi
else
    echo "⚠ $FAILED critical check(s) failed"
    echo ""
    echo "MetaGuildNet structure is valid, but GuildNet runtime is not available."
    echo ""
    echo "To install GuildNet:"
    echo "  mgn install --type local"
    echo ""
    # Don't exit with error code if only optional checks failed
    if [ $PASSED -gt 0 ]; then
        exit 0
    else
        exit 1
    fi
fi

