#!/bin/bash
# Verify network connectivity

echo "Verifying network connectivity..."

check_connectivity() {
    local url="$1"
    local name="$2"
    local optional="${3:-no}"
    
    if curl -s --max-time 5 -k "$url" > /dev/null 2>&1; then
        echo "✓ $name"
        return 0
    else
        if [ "$optional" = "yes" ]; then
            echo "⚠ $name (not installed)"
            return 0
        else
            echo "✗ $name"
            return 1
        fi
    fi
}

FAILED=0
REQUIRED=0

echo "Required connectivity:"
check_connectivity "https://google.com" "Internet connectivity" || { FAILED=$((FAILED + 1)); REQUIRED=$((REQUIRED + 1)); }

echo ""
echo "Optional services:"
check_connectivity "https://localhost:8090/healthz" "GuildNet Host App" "yes"

echo ""
if [ $REQUIRED -eq 0 ]; then
    echo "✅ Required connectivity verified"
    if [ $FAILED -gt 0 ]; then
        echo "⚠  GuildNet Host App not running (install GuildNet to enable)"
    fi
    exit 0
else
    echo "✗ $REQUIRED required connectivity check(s) failed"
    exit 1
fi

