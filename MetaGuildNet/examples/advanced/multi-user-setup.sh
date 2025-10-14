#!/bin/bash
# Advanced Example: Multi-user setup with different workspaces

set -euo pipefail

echo "═══════════════════════════════════════"
echo "  Multi-User Setup Example"
echo "═══════════════════════════════════════"
echo ""

# Configuration
declare -A USERS=(
    [alice]="codercom/code-server:latest"
    [bob]="jupyter/scipy-notebook:latest"
    [charlie]="theiaide/theia-python:latest"
)

created_workspaces=()

echo "Creating workspaces for multiple users..."
echo ""

for user in "${!USERS[@]}"; do
    image="${USERS[$user]}"
    workspace_name="workspace-$user-$(date +%s)"
    
    echo "Creating workspace for $user:"
    echo "  Name:  $workspace_name"
    echo "  Image: $image"
    
    response=$(curl -sk -X POST https://127.0.0.1:8080/api/jobs \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"$workspace_name\",
            \"image\": \"$image\",
            \"env\": [
                {\"name\": \"USER\", \"value\": \"$user\"}
            ]
        }" 2>/dev/null)
    
    if echo "$response" | grep -q '"id"'; then
        workspace_id=$(echo "$response" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
        echo "  ✓ Created: $workspace_id"
        created_workspaces+=("$user:$workspace_id")
    else
        echo "  ✗ Failed to create"
    fi
    
    echo ""
done

if [[ ${#created_workspaces[@]} -eq 0 ]]; then
    echo "No workspaces created. Is Host App running?"
    exit 1
fi

echo "Waiting for workspaces to be ready..."
echo ""

for entry in "${created_workspaces[@]}"; do
    IFS=: read -r user workspace_id <<< "$entry"
    
    echo "Checking $user's workspace..."
    max_wait=60
    waited=0
    
    while [[ $waited -lt $max_wait ]]; do
        status=$(curl -sk "https://127.0.0.1:8080/api/servers/$workspace_id" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || echo "")
        
        if [[ "$status" == "Running" ]]; then
            echo "  ✓ $user: Running"
            echo "    URL: https://127.0.0.1:8080/proxy/server/$workspace_id/"
            break
        fi
        
        sleep 5
        ((waited += 5))
    done
    
    if [[ "$status" != "Running" ]]; then
        echo "  ⚠ $user: Not ready yet (status: $status)"
    fi
done

echo ""
echo "═══════════════════════════════════════"
echo "Summary:"
echo "  Total workspaces: ${#created_workspaces[@]}"
echo ""
echo "To view all workspaces:"
echo "  curl -sk https://127.0.0.1:8080/api/servers | jq"
echo ""
echo "To cleanup all workspaces:"
echo "  curl -sk -X POST https://127.0.0.1:8080/api/admin/stop-all"
echo ""

