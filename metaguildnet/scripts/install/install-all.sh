#!/bin/bash
# Complete installation script for MetaGuildNet/GuildNet
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

echo "===================================="
echo "MetaGuildNet Installation"
echo "===================================="
echo ""

# Check if running with proper permissions
if [ "$EUID" -eq 0 ]; then
    echo "Warning: Running as root. Some steps may require non-root user."
fi

# Step 1: Check prerequisites
echo "Step 1/5: Checking prerequisites..."
bash "$SCRIPT_DIR/00-check-prereqs.sh"

# Step 2: Install microk8s
echo ""
echo "Step 2/5: Installing microk8s..."
bash "$SCRIPT_DIR/01-install-microk8s.sh"

# Step 3: Setup Headscale
echo ""
echo "Step 3/5: Setting up Headscale..."
bash "$SCRIPT_DIR/02-setup-headscale.sh"

# Step 4: Deploy GuildNet
echo ""
echo "Step 4/5: Deploying GuildNet..."
bash "$SCRIPT_DIR/03-deploy-guildnet.sh"

# Step 5: Bootstrap cluster
echo ""
echo "Step 5/5: Bootstrapping cluster..."
bash "$SCRIPT_DIR/04-bootstrap-cluster.sh"

echo ""
echo "===================================="
echo "Installation Complete!"
echo "===================================="
echo ""
echo "GuildNet is now running at: https://localhost:8090"
echo ""
echo "Next steps:"
echo "  1. Verify installation: mgn verify all"
echo "  2. List clusters: mgn cluster list"
echo "  3. Launch dashboard: mgn viz"
echo ""

