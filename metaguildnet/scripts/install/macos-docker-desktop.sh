#!/bin/bash
# Install GuildNet on macOS using Docker Desktop Kubernetes

set -e

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║     GuildNet Installation for macOS (Docker Desktop)         ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}✗ Docker is not running${NC}"
    echo "Please start Docker Desktop and try again"
    exit 1
fi
echo -e "${GREEN}✓ Docker is running${NC}"

# Check if Kubernetes is enabled
if ! kubectl cluster-info > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠ Kubernetes is not enabled in Docker Desktop${NC}"
    echo ""
    echo "To enable Kubernetes:"
    echo "  1. Open Docker Desktop"
    echo "  2. Go to Settings (gear icon)"
    echo "  3. Click 'Kubernetes' in the left sidebar"
    echo "  4. Check 'Enable Kubernetes'"
    echo "  5. Click 'Apply & Restart'"
    echo ""
    echo "After enabling Kubernetes, run this script again."
    exit 1
fi
echo -e "${GREEN}✓ Kubernetes is enabled and running${NC}"

# Get cluster info
echo ""
echo -e "${BLUE}Kubernetes Cluster Information:${NC}"
kubectl cluster-info | head -2

# Check kubectl version
KUBECTL_VERSION=$(kubectl version --client -o json | python3 -c "import sys, json; print(json.load(sys.stdin)['clientVersion']['gitVersion'])")
echo -e "${GREEN}✓ kubectl version: ${KUBECTL_VERSION}${NC}"

# Navigate to GuildNet root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUILDNET_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

echo ""
echo -e "${BLUE}GuildNet directory: ${GUILDNET_ROOT}${NC}"
echo ""

# Create namespace
echo "Creating guildnet namespace..."
if kubectl create namespace guildnet 2>/dev/null; then
    echo -e "${GREEN}✓ Namespace created${NC}"
else
    echo -e "${YELLOW}⚠ Namespace already exists${NC}"
fi

# Check if RethinkDB manifests exist
if [ ! -f "$GUILDNET_ROOT/k8s/rethinkdb.yaml" ]; then
    echo -e "${RED}✗ RethinkDB manifest not found at $GUILDNET_ROOT/k8s/rethinkdb.yaml${NC}"
    exit 1
fi

# Deploy RethinkDB
echo ""
echo "Deploying RethinkDB..."
kubectl apply -f "$GUILDNET_ROOT/k8s/rethinkdb.yaml" -n guildnet

# Wait for RethinkDB to be ready
echo ""
echo "Waiting for RethinkDB to be ready (this may take a minute)..."
kubectl wait --for=condition=ready pod -l app=rethinkdb -n guildnet --timeout=300s || true

# Check RethinkDB status
echo ""
echo -e "${BLUE}RethinkDB Status:${NC}"
kubectl get pods -n guildnet

# Show services
echo ""
echo -e "${BLUE}Services:${NC}"
kubectl get services -n guildnet

echo ""
echo -e "${GREEN}✓ GuildNet infrastructure deployed!${NC}"
echo ""
echo -e "${BLUE}Next steps:${NC}"
echo "  1. Start the GuildNet Host App:"
echo -e "     ${YELLOW}cd $GUILDNET_ROOT${NC}"
echo -e "     ${YELLOW}./scripts/run-hostapp.sh${NC}"
echo ""
echo "  2. Verify installation:"
echo -e "     ${YELLOW}mgn verify all${NC}"
echo ""
echo "  3. Test MetaGuildNet CLI:"
echo -e "     ${YELLOW}mgn cluster list${NC}"
echo -e "     ${YELLOW}mgn viz${NC}"
echo ""
echo -e "${BLUE}Access GuildNet UI at:${NC} https://localhost:8090"
echo ""

