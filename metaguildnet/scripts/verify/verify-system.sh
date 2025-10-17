#!/bin/bash
# Verify system prerequisites

echo "Verifying system prerequisites..."

check_tool() {
    local tool="$1"
    local optional="${2:-no}"
    
    if command -v "$tool" &> /dev/null; then
        echo "✓ $tool"
        return 0
    else
        if [ "$optional" = "yes" ]; then
            echo "⚠ $tool (optional)"
            return 0
        else
            echo "✗ $tool not found"
            return 1
        fi
    fi
}

FAILED=0
REQUIRED=0

# Core tools (required for MetaGuildNet itself)
echo "Required tools:"
check_tool python3 || { FAILED=$((FAILED + 1)); REQUIRED=$((REQUIRED + 1)); }
check_tool go || { FAILED=$((FAILED + 1)); REQUIRED=$((REQUIRED + 1)); }
check_tool curl || { FAILED=$((FAILED + 1)); REQUIRED=$((REQUIRED + 1)); }

echo ""
echo "Optional tools (for full GuildNet installation):"
check_tool kubectl "yes"
check_tool docker "yes"
check_tool jq "yes"

echo ""
if [ $REQUIRED -eq 0 ]; then
    echo "✅ All required tools present"
    if [ $FAILED -gt 0 ]; then
        echo "⚠  Some optional tools missing (install them for full GuildNet support)"
    fi
    exit 0
else
    echo "✗ $REQUIRED required tool(s) missing"
    exit 1
fi

