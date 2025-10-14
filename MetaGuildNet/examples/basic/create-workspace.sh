#!/bin/bash
# Example: Create a basic workspace

set -euo pipefail

# Support dev mode
DEV_MODE="${METAGN_DEV_MODE:-false}"

echo "═══════════════════════════════════════"
echo "  Create Basic Workspace Example"
echo "═══════════════════════════════════════"
echo ""

if [[ "$DEV_MODE" == "true" ]]; then
    echo "Running in DEV MODE - demonstrating workspace creation flow"
    echo ""
    
    # Configuration
    WORKSPACE_NAME="example-workspace-$(date +%s)"
    WORKSPACE_IMAGE="codercom/code-server:latest"
    
    echo "Would create workspace:"
    echo "  Name:  $WORKSPACE_NAME"
    echo "  Image: $WORKSPACE_IMAGE"
    echo ""
    
    echo "✓ Example demonstrates:"
    echo "  1. Workspace creation via API"
    echo "  2. Status polling"
    echo "  3. Access URL generation"
    echo "  4. Cleanup instructions"
    echo ""
    
    echo "To run this example for real:"
    echo "  1. Start Host App: make run"
    echo "  2. Run: bash MetaGuildNet/examples/basic/create-workspace.sh"
    echo ""
    exit 0
fi

# Configuration
WORKSPACE_NAME="example-workspace-$(date +%s)"
WORKSPACE_IMAGE="codercom/code-server:latest"

echo "Creating workspace:"
echo "  Name:  $WORKSPACE_NAME"
echo "  Image: $WORKSPACE_IMAGE"
echo ""

# Create workspace
echo "Sending request..."
response=$(curl -sk -X POST https://127.0.0.1:8080/api/jobs \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"$WORKSPACE_NAME\",
        \"image\": \"$WORKSPACE_IMAGE\",
        \"env\": [
            {\"name\": \"PASSWORD\", \"value\": \"example123\"}
        ]
    }")

# Parse response
if echo "$response" | grep -q '"id"'; then
    workspace_id=$(echo "$response" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
    echo "✓ Workspace created successfully!"
    echo "  ID: $workspace_id"
    echo ""
    
    echo "Waiting for workspace to be ready..."
    max_wait=120
    waited=0
    
    while [[ $waited -lt $max_wait ]]; do
        status=$(curl -sk "https://127.0.0.1:8080/api/servers/$workspace_id" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || echo "")
        
        if [[ "$status" == "Running" ]]; then
            echo "✓ Workspace is running!"
            echo ""
            echo "Access your workspace:"
            echo "  URL: https://127.0.0.1:8080/proxy/server/$workspace_id/"
            echo "  Password: example123"
            echo ""
            echo "To delete this workspace:"
            echo "  curl -sk -X DELETE https://127.0.0.1:8080/api/servers/$workspace_id"
            echo ""
            exit 0
        fi
        
        echo "  Status: $status (waited ${waited}s)"
        sleep 5
        waited=$((waited + 5))
    done
    
    echo "✗ Workspace did not become ready within ${max_wait}s"
    echo "  Check status: curl -sk https://127.0.0.1:8080/api/servers/$workspace_id"
    exit 1
else
    echo "✗ Failed to create workspace"
    echo "  Response: $response"
    echo ""
    echo "Is Host App running?"
    echo "  Check: curl -sk https://127.0.0.1:8080/healthz"
    exit 1
fi

