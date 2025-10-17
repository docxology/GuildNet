# MetaGuildNet Examples

Detailed walkthroughs for common scenarios using MetaGuildNet.

## Example 1: Basic Cluster and Workspace Management

### Scenario

Set up a new cluster and deploy a code-server workspace for development.

### Steps

#### 1. Verify Prerequisites

```bash
mgn verify system
mgn verify kubernetes
```

#### 2. Bootstrap Cluster

```bash
# Using automated installer
mgn install --type local --cluster-name dev-cluster

# Or manually if GuildNet already installed
mgn cluster list
```

#### 3. Create Development Workspace

```bash
mgn workspace create dev-cluster \
  --name my-codeserver \
  --image codercom/code-server:4.90.3 \
  --env PASSWORD=mydevpassword \
  --port 8080
```

#### 4. Wait for Workspace Ready

```bash
mgn workspace wait dev-cluster my-codeserver --timeout 5m
```

#### 5. Access Workspace

```bash
# Get proxy URL
mgn workspace url dev-cluster my-codeserver

# Opens in browser
mgn workspace open dev-cluster my-codeserver
```

### Go SDK Equivalent

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
    "github.com/docxology/GuildNet/metaguildnet/sdk/go/testing"
)

func main() {
    c := client.NewClient("https://localhost:8090", "")
    ctx := context.Background()
    
    // List clusters
    clusters, err := c.Clusters().List(ctx)
    if err != nil {
        log.Fatal(err)
    }
    clusterID := clusters[0].ID
    
    // Create workspace
    spec := client.WorkspaceSpec{
        Name:  "my-codeserver",
        Image: "codercom/code-server:4.90.3",
        Env: []client.EnvVar{
            {Name: "PASSWORD", Value: "mydevpassword"},
        },
        Ports: []client.Port{
            {ContainerPort: 8080, Name: "http"},
        },
    }
    
    ws, err := c.Workspaces(clusterID).Create(ctx, spec)
    if err != nil {
        log.Fatal(err)
    }
    
    // Wait for ready
    err = testing.WaitForWorkspaceReady(c, clusterID, ws.Name, 5*time.Minute)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Workspace ready: %s\n", ws.Name)
}
```

## Example 2: Multi-Cluster Deployment

### Scenario

Deploy the same application to multiple clusters for geographic distribution.

### Configuration

Create `multi-cluster.yaml`:

```yaml
deployment:
  name: web-app
  image: myregistry/webapp:v1.0.0
  clusters:
    - id: us-east-1
      replicas: 3
    - id: us-west-2
      replicas: 3
    - id: eu-west-1
      replicas: 2
  env:
    - name: DATABASE_URL
      value: postgres://db.example.com/prod
    - name: REGION
      valueFrom: cluster  # Inject cluster ID
```

### Deployment Script

```bash
#!/bin/bash
# deploy-multi-cluster.sh

CONFIG_FILE=${1:-multi-cluster.yaml}

# Parse YAML and deploy to each cluster
yq eval '.deployment.clusters[].id' "$CONFIG_FILE" | while read cluster_id; do
  echo "Deploying to $cluster_id..."
  
  image=$(yq eval '.deployment.image' "$CONFIG_FILE")
  name=$(yq eval '.deployment.name' "$CONFIG_FILE")
  
  mgn workspace create "$cluster_id" \
    --name "$name" \
    --image "$image" \
    --env DATABASE_URL=postgres://db.example.com/prod \
    --env REGION="$cluster_id"
  
  mgn workspace wait "$cluster_id" "$name" --timeout 5m
done

echo "Deployment complete to all clusters"
```

### Go SDK Approach

See `orchestrator/examples/multi-cluster/load-balance.go` for a complete implementation with health checking and rollback.

## Example 3: Rolling Update

### Scenario

Update an application across multiple clusters with zero downtime using rolling updates.

### Strategy

1. Deploy new version alongside old version
2. Gradually shift traffic to new version
3. Monitor health
4. Complete rollout or rollback if issues detected

### Implementation

```bash
#!/bin/bash
# rolling-update.sh

CLUSTER_ID=$1
WORKSPACE_NAME=$2
NEW_IMAGE=$3

echo "Starting rolling update of $WORKSPACE_NAME to $NEW_IMAGE"

# Get current workspace
CURRENT=$(mgn workspace get "$CLUSTER_ID" "$WORKSPACE_NAME" -o json)

# Create new workspace with new image (blue-green style)
NEW_NAME="${WORKSPACE_NAME}-new"
mgn workspace create "$CLUSTER_ID" \
  --name "$NEW_NAME" \
  --image "$NEW_IMAGE" \
  --env-from "$WORKSPACE_NAME"

# Wait for new workspace
mgn workspace wait "$CLUSTER_ID" "$NEW_NAME" --timeout 10m

# Run smoke tests
if ! mgn workspace test "$CLUSTER_ID" "$NEW_NAME" --http-check /health; then
  echo "Health check failed, rolling back"
  mgn workspace delete "$CLUSTER_ID" "$NEW_NAME"
  exit 1
fi

# Swap: delete old, rename new
mgn workspace delete "$CLUSTER_ID" "$WORKSPACE_NAME"
sleep 5
mgn workspace rename "$CLUSTER_ID" "$NEW_NAME" "$WORKSPACE_NAME"

echo "Rolling update complete"
```

See `orchestrator/examples/lifecycle/rolling-update.sh` for production-ready version.

## Example 4: Database Operations

### Scenario

Create a database, define schema, and populate with initial data.

### Using Python CLI

```bash
# Create database
mgn db create my-cluster my-app-db --description "Application database"

# Create table
mgn db table create my-cluster my-app-db users \
  --schema '
    name:string:required,
    email:string:required:unique,
    created_at:timestamp
  ' \
  --primary-key id

# Insert data
mgn db insert my-cluster my-app-db users \
  --data '{"name":"Alice","email":"alice@example.com"}'

# Query data
mgn db query my-cluster my-app-db users --limit 10
```

### Using Go SDK

```go
package main

import (
    "context"
    "log"
    
    "github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
    "github.com/docxology/GuildNet/internal/model"
)

func main() {
    c := client.NewClient("https://localhost:8090", "")
    ctx := context.Background()
    clusterID := "my-cluster"
    
    // Create database
    db, err := c.Databases(clusterID).Create(ctx, "my-app-db", "Application database")
    if err != nil {
        log.Fatal(err)
    }
    
    // Define schema
    table := model.Table{
        Name:       "users",
        PrimaryKey: "id",
        Schema: []model.ColumnDef{
            {Name: "name", Type: "string", Required: true},
            {Name: "email", Type: "string", Required: true, Unique: true},
            {Name: "created_at", Type: "timestamp"},
        },
    }
    
    err = c.Databases(clusterID).CreateTable(ctx, db.ID, table)
    if err != nil {
        log.Fatal(err)
    }
    
    // Insert data
    rows := []map[string]any{
        {"name": "Alice", "email": "alice@example.com"},
        {"name": "Bob", "email": "bob@example.com"},
    }
    
    ids, err := c.Databases(clusterID).InsertRows(ctx, db.ID, "users", rows)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Inserted %d rows", len(ids))
    
    // Query data
    results, _, err := c.Databases(clusterID).Query(ctx, db.ID, "users", "", 10, "", true)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Query returned %d rows", len(results))
}
```

## Example 5: CI/CD Integration

### Scenario

Integrate GuildNet deployments into a GitHub Actions workflow.

### GitHub Actions Workflow

```yaml
# .github/workflows/deploy.yml
name: Deploy to GuildNet

on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install MetaGuildNet CLI
        run: |
          cd metaguildnet/python
          pip install uv
          uv pip install -e .
      
      - name: Configure GuildNet
        env:
          GUILDNET_API_URL: ${{ secrets.GUILDNET_API_URL }}
          GUILDNET_API_TOKEN: ${{ secrets.GUILDNET_API_TOKEN }}
        run: |
          mgn cluster list
      
      - name: Build and Push Image
        run: |
          docker build -t myregistry/app:${{ github.sha }} .
          docker push myregistry/app:${{ github.sha }}
      
      - name: Deploy to Staging
        env:
          CLUSTER_ID: staging
        run: |
          mgn workspace create $CLUSTER_ID \
            --name myapp-staging \
            --image myregistry/app:${{ github.sha }} \
            --update-if-exists
          
          mgn workspace wait $CLUSTER_ID myapp-staging --timeout 5m
      
      - name: Run Smoke Tests
        run: |
          mgn workspace test staging myapp-staging \
            --http-check /health \
            --http-check /api/status
      
      - name: Deploy to Production
        if: success()
        run: |
          for cluster in prod-us-east prod-us-west prod-eu-west; do
            mgn workspace create $cluster \
              --name myapp \
              --image myregistry/app:${{ github.sha }} \
              --update-if-exists
          done
      
      - name: Verify Production
        run: |
          for cluster in prod-us-east prod-us-west prod-eu-west; do
            mgn workspace wait $cluster myapp --timeout 10m
            mgn workspace test $cluster myapp --http-check /health
          done
```

See `orchestrator/examples/cicd/github-actions.yaml` for full example with rollback.

## Example 6: Real-time Monitoring Dashboard

### Scenario

Monitor cluster health and workspace status in real-time.

### Launch Dashboard

```bash
mgn viz
```

### Custom Monitoring Script

```python
#!/usr/bin/env python3
# monitor.py

from metaguildnet.api.client import Client
from rich.live import Live
from rich.table import Table
import time

def generate_table(client):
    table = Table(title="GuildNet Cluster Status")
    table.add_column("Cluster", style="cyan")
    table.add_column("Status", style="green")
    table.add_column("Workspaces", justify="right")
    table.add_column("Health")
    
    for cluster in client.clusters.list():
        health = client.health.cluster(cluster.id)
        workspaces = client.workspaces(cluster.id).list()
        
        status = "✓" if health.k8s_reachable else "✗"
        table.add_row(
            cluster.name,
            status,
            str(len(workspaces)),
            "Healthy" if health.k8s_reachable else "Unhealthy"
        )
    
    return table

def main():
    client = Client("https://localhost:8090")
    
    with Live(generate_table(client), refresh_per_second=1) as live:
        while True:
            time.sleep(5)
            live.update(generate_table(client))

if __name__ == "__main__":
    main()
```

## Example 7: Backup and Restore

### Backup Configuration

```bash
#!/bin/bash
# backup-cluster.sh

CLUSTER_ID=$1
BACKUP_DIR="backups/$(date +%Y%m%d-%H%M%S)"

mkdir -p "$BACKUP_DIR"

# Backup cluster settings
mgn cluster get "$CLUSTER_ID" -o yaml > "$BACKUP_DIR/cluster-settings.yaml"

# Backup all workspaces
mgn workspace list "$CLUSTER_ID" -o yaml > "$BACKUP_DIR/workspaces.yaml"

# Backup databases
mgn db list "$CLUSTER_ID" | while read db_id; do
  mgn db export "$CLUSTER_ID" "$db_id" \
    --format json \
    --output "$BACKUP_DIR/db-${db_id}.json"
done

echo "Backup complete: $BACKUP_DIR"
```

### Restore Configuration

```bash
#!/bin/bash
# restore-cluster.sh

BACKUP_DIR=$1
NEW_CLUSTER_ID=$2

# Restore cluster settings
mgn cluster apply -f "$BACKUP_DIR/cluster-settings.yaml" --id "$NEW_CLUSTER_ID"

# Restore workspaces
yq eval '.workspaces[]' "$BACKUP_DIR/workspaces.yaml" | while read -r ws; do
  mgn workspace apply "$NEW_CLUSTER_ID" -f - <<< "$ws"
done

# Restore databases
for db_file in "$BACKUP_DIR"/db-*.json; do
  db_id=$(basename "$db_file" .json | sed 's/^db-//')
  mgn db import "$NEW_CLUSTER_ID" "$db_id" --file "$db_file"
done

echo "Restore complete"
```

## Example 8: Testing Workflows

### Unit Test with Go SDK

```go
package myapp_test

import (
    "context"
    "testing"
    
    "github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
    "github.com/docxology/GuildNet/metaguildnet/sdk/go/testing"
)

func TestWorkspaceDeployment(t *testing.T) {
    c := client.NewClient("https://localhost:8090", "")
    ctx := context.Background()
    
    // Use test cluster
    tc := testing.NewTestCluster(t)
    defer tc.Cleanup()
    
    // Create workspace
    spec := client.WorkspaceSpec{
        Name:  "test-ws",
        Image: "nginx:alpine",
    }
    
    ws, err := c.Workspaces(tc.ID).Create(ctx, spec)
    if err != nil {
        t.Fatalf("failed to create workspace: %v", err)
    }
    
    // Assert workspace is healthy
    testing.AssertWorkspaceRunning(t, c, tc.ID, ws.Name)
}
```

### Integration Test Script

```bash
#!/bin/bash
# test-integration.sh

set -e

echo "Running integration tests..."

# Setup test cluster
TEST_CLUSTER=$(mgn cluster create-test --name test-integration)
trap "mgn cluster delete $TEST_CLUSTER" EXIT

# Test workspace creation
mgn workspace create "$TEST_CLUSTER" --name test-nginx --image nginx:alpine
mgn workspace wait "$TEST_CLUSTER" test-nginx --timeout 2m

# Test workspace is accessible
mgn workspace test "$TEST_CLUSTER" test-nginx --http-check /

# Test database operations
mgn db create "$TEST_CLUSTER" testdb
mgn db table create "$TEST_CLUSTER" testdb testtable --schema "name:string"
mgn db insert "$TEST_CLUSTER" testdb testtable --data '{"name":"test"}'
mgn db query "$TEST_CLUSTER" testdb testtable | grep -q "test"

echo "All integration tests passed"
```

## More Examples

See the `orchestrator/examples/` directory for additional production-ready examples:

- Multi-cluster federation
- Blue-green deployments
- Canary releases
- A/B testing
- Disaster recovery
- Automated scaling

## Next Steps

- Review [API Reference](api-reference.md) for complete SDK documentation
- Explore [concepts](concepts.md) for architectural patterns
- Check [orchestrator examples](../orchestrator/examples/) for full implementations

