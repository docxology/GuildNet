#!/usr/bin/env bash
set -euo pipefail

#
# MetaGuildNet Comprehensive Method Testing
#
# Tests all MetaGuildNet methods and functionality including:
# - Configuration management
# - Workflow execution
# - Error handling
# - Output generation
# - Visualization
# - Validation
#

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
METAGUILDNET_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_ROOT="$(cd "$METAGUILDNET_DIR/.." && pwd)"

# Colors
RED='\033[91m'
GREEN='\033[92m'
YELLOW='\033[93m'
CYAN='\033[96m'
BOLD='\033[1m'
RESET='\033[0m'

# Counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Test result tracking
declare -a FAILED_TESTS=()

echo -e "${BOLD}${CYAN}"
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║        METAGUILDNET COMPREHENSIVE METHOD TESTING              ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo -e "${RESET}"

# Test helper functions
test_method() {
    local test_name="$1"
    local test_command="$2"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    echo -ne "  Testing: ${test_name}... "
    
    if eval "$test_command" &>/dev/null; then
        echo -e "${GREEN}✅ PASS${RESET}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}❌ FAIL${RESET}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_TESTS+=("$test_name")
        return 1
    fi
}

test_method_with_output() {
    local test_name="$1"
    local test_command="$2"
    local expected_output="$3"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    echo -ne "  Testing: ${test_name}... "
    
    output=$(eval "$test_command" 2>&1)
    if echo "$output" | grep -q "$expected_output"; then
        echo -e "${GREEN}✅ PASS${RESET}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}❌ FAIL${RESET}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_TESTS+=("$test_name")
        return 1
    fi
}

# Test Suite 1: Configuration Methods
echo -e "\n${CYAN}📋 Testing Configuration Methods${RESET}"
test_method "Config file exists" "test -f $METAGUILDNET_DIR/config.json"
test_method "Config is valid JSON" "python3 -m json.tool < $METAGUILDNET_DIR/config.json >/dev/null"
test_method "Dev config exists" "test -f $METAGUILDNET_DIR/dev-config.json"
test_method "Dev config is valid JSON" "python3 -m json.tool < $METAGUILDNET_DIR/dev-config.json >/dev/null"
test_method_with_output "Config has meta_setup" "cat $METAGUILDNET_DIR/config.json" "meta_setup"
test_method_with_output "Config has verification" "cat $METAGUILDNET_DIR/config.json" "verification"
test_method_with_output "Config has testing" "cat $METAGUILDNET_DIR/config.json" "testing"
test_method_with_output "Config has examples" "cat $METAGUILDNET_DIR/config.json" "examples"
test_method_with_output "Config has diagnostics" "cat $METAGUILDNET_DIR/config.json" "diagnostics"
test_method_with_output "Config has logging" "cat $METAGUILDNET_DIR/config.json" "logging"

# Test Suite 2: Python Runner Methods
echo -e "\n${CYAN}🐍 Testing Python Runner Methods${RESET}"
test_method "run.py exists" "test -f $METAGUILDNET_DIR/run.py"
test_method "run.py has shebang" "head -1 $METAGUILDNET_DIR/run.py | grep -q '^#!/usr/bin/env python3'"
test_method_with_output "run.py --help works" "python3 $METAGUILDNET_DIR/run.py --help" "MetaGuildNet Runner"
test_method_with_output "run.py --dry-run works" "python3 $METAGUILDNET_DIR/run.py --dry-run" "Configuration"
test_method_with_output "run.py supports --config" "python3 $METAGUILDNET_DIR/run.py --help" "config"
test_method_with_output "run.py supports --workflow" "python3 $METAGUILDNET_DIR/run.py --help" "workflow"
test_method_with_output "run.py supports --log-level" "python3 $METAGUILDNET_DIR/run.py --help" "log-level"

# Test Suite 3: Visualization Methods
echo -e "\n${CYAN}🎨 Testing Visualization Methods${RESET}"
test_method "visualize.py exists" "test -f $METAGUILDNET_DIR/visualize.py"
test_method "visualize.py executable" "test -x $METAGUILDNET_DIR/visualize.py"
test_method_with_output "visualize.py --help works" "python3 $METAGUILDNET_DIR/visualize.py --help" "Visualization"
test_method_with_output "visualize.py runs" "python3 $METAGUILDNET_DIR/visualize.py 2>&1" "DASHBOARD"
test_method "Visual report generated" "test -f $METAGUILDNET_DIR/VISUAL_REPORT.txt"
test_method_with_output "Visual report has dashboard" "cat $METAGUILDNET_DIR/VISUAL_REPORT.txt" "DASHBOARD"
test_method_with_output "Visual report has timeline" "cat $METAGUILDNET_DIR/VISUAL_REPORT.txt" "TIMELINE"
test_method_with_output "Visual report has matrix" "cat $METAGUILDNET_DIR/VISUAL_REPORT.txt" "FEATURE MATRIX"

# Test Suite 4: Validation Methods
echo -e "\n${CYAN}✅ Testing Validation Methods${RESET}"
test_method "validate.py exists" "test -f $METAGUILDNET_DIR/validate.py"
test_method "validate.py executable" "test -x $METAGUILDNET_DIR/validate.py"
test_method_with_output "validate.py --help works" "python3 $METAGUILDNET_DIR/validate.py --help" "Validation"
test_method_with_output "validate.py --quick works" "python3 $METAGUILDNET_DIR/validate.py --quick" "Validating"
test_method "Validation has high pass rate" "python3 $METAGUILDNET_DIR/validate.py 2>&1 | grep -E 'Success Rate: [89][0-9]\.[0-9]%|Success Rate: 100\.0%'"

# Test Suite 5: Output Generation Methods
echo -e "\n${CYAN}📊 Testing Output Generation Methods${RESET}"
test_method "Outputs directory exists" "test -d $METAGUILDNET_DIR/outputs"
test_method "Verification output exists" "test -f $METAGUILDNET_DIR/outputs/verification_output.txt"
test_method "Diagnostics output exists" "test -f $METAGUILDNET_DIR/outputs/diagnostics_output.txt"
test_method "Testing output exists" "test -f $METAGUILDNET_DIR/outputs/testing_output.txt"
test_method "Examples output exists" "test -f $METAGUILDNET_DIR/outputs/examples_output.txt"
test_method "Config display exists" "test -f $METAGUILDNET_DIR/outputs/configuration_display.txt"
test_method "Help display exists" "test -f $METAGUILDNET_DIR/outputs/help_display.txt"
test_method "All outputs non-empty" "test $(find $METAGUILDNET_DIR/outputs -type f -size 0 | wc -l) -eq 0"

# Test Suite 6: Report Generation Methods
echo -e "\n${CYAN}📋 Testing Report Generation Methods${RESET}"
test_method "Reports directory exists" "test -d $METAGUILDNET_DIR/reports"
test_method "Execution report exists" "test -f $METAGUILDNET_DIR/reports/EXECUTION_REPORT.md"
test_method "Output summary exists" "test -f $METAGUILDNET_DIR/reports/OUTPUT_SUMMARY.md"
test_method "Execution report substantial" "test $(wc -c < $METAGUILDNET_DIR/reports/EXECUTION_REPORT.md) -gt 5000"
test_method "Output summary substantial" "test $(wc -c < $METAGUILDNET_DIR/reports/OUTPUT_SUMMARY.md) -gt 3000"

# Test Suite 7: Visualization Elements
echo -e "\n${CYAN}🎨 Testing Visualization Elements${RESET}"
test_method_with_output "Outputs have Unicode box drawing" "cat $METAGUILDNET_DIR/outputs/verification_output.txt" "╔"
test_method_with_output "Outputs have ANSI colors" "cat $METAGUILDNET_DIR/outputs/verification_output.txt" "\[96m"
test_method_with_output "Outputs have status indicators" "cat $METAGUILDNET_DIR/outputs/verification_output.txt" "✗"
test_method_with_output "Outputs have timestamps" "cat $METAGUILDNET_DIR/outputs/verification_output.txt" "\[0[0-9]:[0-9][0-9]:[0-9][0-9]\]"
test_method_with_output "Outputs have emojis" "cat $METAGUILDNET_DIR/outputs/examples_output.txt" "💡"

# Test Suite 8: Error Handling Methods
echo -e "\n${CYAN}🔧 Testing Error Handling Methods${RESET}"
test_method_with_output "Verification shows errors gracefully" "cat $METAGUILDNET_DIR/outputs/verification_output.txt" "UNHEALTHY"
test_method_with_output "Verification provides solutions" "cat $METAGUILDNET_DIR/outputs/verification_output.txt" "Common issues"
test_method_with_output "Testing shows troubleshooting" "cat $METAGUILDNET_DIR/outputs/testing_output.txt" "troubleshooting"
test_method_with_output "Examples show prerequisites" "cat $METAGUILDNET_DIR/outputs/examples_output.txt" "Prerequisites"
test_method_with_output "Examples provide commands" "cat $METAGUILDNET_DIR/outputs/examples_output.txt" "make run"

# Test Suite 9: Script Integration Methods
echo -e "\n${CYAN}📜 Testing Script Integration Methods${RESET}"
test_method "Setup wizard exists" "test -f $METAGUILDNET_DIR/scripts/setup/setup_wizard.sh"
test_method "Verify all script exists" "test -f $METAGUILDNET_DIR/scripts/verify/verify_all.sh"
test_method "Diagnose script exists" "test -f $METAGUILDNET_DIR/scripts/utils/diagnose.sh"
test_method "Scripts are executable" "test -x $METAGUILDNET_DIR/scripts/setup/setup_wizard.sh"
test_method "Test framework exists" "test -f $METAGUILDNET_DIR/tests/lib/test_framework.sh"

# Test Suite 10: Makefile Integration Methods
echo -e "\n${CYAN}⚙️ Testing Makefile Integration Methods${RESET}"
test_method "Makefile exists" "test -f $METAGUILDNET_DIR/Makefile"
test_method_with_output "Makefile has meta-setup" "grep -q 'meta-setup' $METAGUILDNET_DIR/Makefile" "meta-setup"
test_method_with_output "Makefile has meta-verify" "grep -q 'meta-verify' $METAGUILDNET_DIR/Makefile" "meta-verify"
test_method_with_output "Makefile has meta-test" "grep -q 'meta-test' $METAGUILDNET_DIR/Makefile" "meta-test"
test_method_with_output "Makefile has meta-diagnose" "grep -q 'meta-diagnose' $METAGUILDNET_DIR/Makefile" "meta-diagnose"

# Print Summary
echo -e "\n${BOLD}════════════════════════════════════════════════════════════════${RESET}"
echo -e "${BOLD}TEST SUMMARY${RESET}"
echo "════════════════════════════════════════════════════════════════"
echo -e "  ${GREEN}Passed:${RESET} $TESTS_PASSED"
echo -e "  ${RED}Failed:${RESET} $TESTS_FAILED"
echo "  Total:  $TESTS_RUN"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "\n  ${GREEN}${BOLD}✅ ALL TESTS PASSED${RESET}"
    success_rate="100.0"
else
    echo -e "\n  ${YELLOW}⚠ SOME TESTS FAILED${RESET}"
    success_rate=$(awk "BEGIN {printf \"%.1f\", ($TESTS_PASSED / $TESTS_RUN) * 100}")
    
    echo -e "\n${BOLD}Failed Tests:${RESET}"
    for test in "${FAILED_TESTS[@]}"; do
        echo "  - $test"
    done
fi

echo "  Success Rate: ${success_rate}%"
echo "════════════════════════════════════════════════════════════════"

# Exit with appropriate code
if [ $TESTS_FAILED -eq 0 ]; then
    exit 0
else
    exit 1
fi

