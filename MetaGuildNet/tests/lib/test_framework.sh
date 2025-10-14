#!/bin/bash
# Test framework for MetaGuildNet

# Colors
readonly TEST_GREEN='\033[0;32m'
readonly TEST_RED='\033[0;31m'
readonly TEST_YELLOW='\033[1;33m'
readonly TEST_RESET='\033[0m'

# Test state
TEST_SUITE_NAME=""
declare -a TEST_RESULTS=()
TEST_PASSED=0
TEST_FAILED=0

# Test suite
test_suite() {
    TEST_SUITE_NAME="$1"
    echo -e "\n${TEST_YELLOW}═══ Test Suite: $TEST_SUITE_NAME ═══${TEST_RESET}\n"
}

# Test case
test_case() {
    local test_name="$1"
    echo -e "${TEST_YELLOW}► Test: $test_name${TEST_RESET}"
}

# Assertions
assert_equals() {
    local actual="$1"
    local expected="$2"
    local message="${3:-Assertion failed}"
    
    if [[ "$actual" == "$expected" ]]; then
        test_pass "$message"
        return 0
    else
        test_fail "$message (expected: '$expected', actual: '$actual')"
        return 1
    fi
}

assert_not_equals() {
    local actual="$1"
    local unexpected="$2"
    local message="${3:-Assertion failed}"
    
    if [[ "$actual" != "$unexpected" ]]; then
        test_pass "$message"
        return 0
    else
        test_fail "$message (unexpected value: '$unexpected')"
        return 1
    fi
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local message="${3:-Assertion failed}"
    
    if [[ "$haystack" == *"$needle"* ]]; then
        test_pass "$message"
        return 0
    else
        test_fail "$message (expected to contain: '$needle')"
        return 1
    fi
}

assert_true() {
    local condition="$1"
    local message="${2:-Assertion failed}"
    
    if eval "$condition"; then
        test_pass "$message"
        return 0
    else
        test_fail "$message (condition was false)"
        return 1
    fi
}

assert_file_exists() {
    local file="$1"
    local message="${2:-File does not exist}"
    
    if [[ -f "$file" ]]; then
        test_pass "$message"
        return 0
    else
        test_fail "$message: $file"
        return 1
    fi
}

assert_command_succeeds() {
    local message="$1"
    shift
    local cmd=("$@")
    
    if "${cmd[@]}" &>/dev/null; then
        test_pass "$message"
        return 0
    else
        test_fail "$message (command failed: ${cmd[*]})"
        return 1
    fi
}

# Test result tracking
test_pass() {
    local message="$1"
    echo -e "  ${TEST_GREEN}✓${TEST_RESET} $message"
    TEST_RESULTS+=("PASS:$message")
    TEST_PASSED=$((TEST_PASSED + 1))
}

test_fail() {
    local message="$1"
    echo -e "  ${TEST_RED}✗${TEST_RESET} $message"
    TEST_RESULTS+=("FAIL:$message")
    TEST_FAILED=$((TEST_FAILED + 1))
}

# Test suite summary
run_test_suite() {
    local total=$((TEST_PASSED + TEST_FAILED))
    
    echo ""
    echo -e "${TEST_YELLOW}═══════════════════════════════════════${TEST_RESET}"
    echo -e "${TEST_YELLOW}Test Suite: $TEST_SUITE_NAME${TEST_RESET}"
    echo -e "${TEST_YELLOW}═══════════════════════════════════════${TEST_RESET}"
    echo -e "Total:  $total"
    echo -e "${TEST_GREEN}Passed: $TEST_PASSED${TEST_RESET}"
    
    if [[ $TEST_FAILED -gt 0 ]]; then
        echo -e "${TEST_RED}Failed: $TEST_FAILED${TEST_RESET}"
    fi
    
    echo ""
    
    if [[ $TEST_FAILED -eq 0 ]]; then
        echo -e "${TEST_GREEN}All tests passed!${TEST_RESET}"
        return 0
    else
        echo -e "${TEST_RED}Some tests failed${TEST_RESET}"
        return 1
    fi
}

# Export functions
export -f test_suite test_case
export -f assert_equals assert_not_equals assert_contains assert_true
export -f assert_file_exists assert_command_succeeds
export -f test_pass test_fail run_test_suite

