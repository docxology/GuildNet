#!/bin/bash
# Check system prerequisites for GuildNet installation

set -e

echo "Checking system prerequisites..."
echo ""

MISSING_TOOLS=()

# Check required tools
check_tool() {
    if command -v "$1" &> /dev/null; then
        echo "✓ $1 found"
    else
        echo "✗ $1 not found"
        MISSING_TOOLS+=("$1")
    fi
}

check_tool "kubectl"
check_tool "docker"
check_tool "curl"
check_tool "jq"
check_tool "bash"

# Check for snap (for microk8s)
if command -v snap &> /dev/null; then
    echo "✓ snap found"
else
    echo "⚠ snap not found (required for microk8s)"
    MISSING_TOOLS+=("snap")
fi

# Check system resources
echo ""
echo "Checking system resources..."

# Check available memory
if [ -f /proc/meminfo ]; then
    MEM_TOTAL=$(grep MemTotal /proc/meminfo | awk '{print $2}')
    MEM_GB=$((MEM_TOTAL / 1024 / 1024))
    if [ "$MEM_GB" -ge 4 ]; then
        echo "✓ Memory: ${MEM_GB}GB (sufficient)"
    else
        echo "⚠ Memory: ${MEM_GB}GB (4GB+ recommended)"
    fi
fi

# Check available disk space
DISK_FREE=$(df -BG / | tail -1 | awk '{print $4}' | sed 's/G//')
if [ "$DISK_FREE" -ge 20 ]; then
    echo "✓ Disk space: ${DISK_FREE}GB (sufficient)"
else
    echo "⚠ Disk space: ${DISK_FREE}GB (20GB+ recommended)"
fi

# Check if ports are available
check_port() {
    if ! lsof -i :"$1" &> /dev/null; then
        echo "✓ Port $1 is available"
    else
        echo "⚠ Port $1 is in use"
    fi
}

echo ""
echo "Checking required ports..."
check_port 8090  # GuildNet Host App
check_port 6443  # Kubernetes API (microk8s)
check_port 443   # Headscale

# Summary
echo ""
if [ ${#MISSING_TOOLS[@]} -eq 0 ]; then
    echo "✓ All prerequisites met"
    exit 0
else
    echo "✗ Missing required tools: ${MISSING_TOOLS[*]}"
    echo ""
    echo "Please install missing tools before continuing:"
    for tool in "${MISSING_TOOLS[@]}"; do
        case "$tool" in
            snap)
                echo "  - Install snap: https://snapcraft.io/docs/installing-snapd"
                ;;
            *)
                echo "  - Install $tool"
                ;;
        esac
    done
    exit 1
fi

