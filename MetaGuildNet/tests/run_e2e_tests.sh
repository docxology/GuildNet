#!/bin/bash
# Run all end-to-end tests

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Running MetaGuildNet End-to-End Tests"
echo "======================================"
echo ""

total_passed=0
total_failed=0

for test in "$SCRIPT_DIR"/e2e/*_test.sh "$SCRIPT_DIR"/e2e/*.sh; do
    if [[ -f "$test" ]]; then
        echo "Running: $(basename "$test")"
        if bash "$test"; then
            total_passed=$((total_passed + 1))
        else
            total_failed=$((total_failed + 1))
        fi
        echo ""
    fi
done

echo "======================================"
echo "End-to-End Test Summary"
echo "  Passed: $total_passed"
echo "  Failed: $total_failed"
echo "======================================"

[[ $total_failed -eq 0 ]] && exit 0 || exit 1

