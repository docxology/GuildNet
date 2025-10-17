# MetaGuildNet Quick Start

Get up and running with MetaGuildNet in minutes.

## Installation

### 1. Install Python CLI

```bash
cd metaguildnet/python
uv pip install -e .
```

### 2. Verify Installation

```bash
mgn version
```

## Basic Usage

### Check System

```bash
# Verify prerequisites
mgn verify system

# Check network
mgn verify network

# Verify Kubernetes
mgn verify kubernetes

# Verify GuildNet
mgn verify guildnet

# Run all checks
mgn verify all
```

### Manage Clusters

```bash
# List clusters
mgn cluster list

# Get cluster details
mgn cluster get <cluster-id>

# Check cluster health
mgn cluster status <cluster-id>
```

### Manage Workspaces

```bash
# List workspaces
mgn workspace list <cluster-id>

# Create workspace
mgn workspace create <cluster-id> \
  --name my-workspace \
  --image nginx:alpine

# Wait for ready
mgn workspace wait <cluster-id> my-workspace

# View logs
mgn workspace logs <cluster-id> my-workspace

# Delete workspace
mgn workspace delete <cluster-id> my-workspace
```

### Database Operations

```bash
# Create database
mgn db create <cluster-id> mydb

# Create table
mgn db table create <cluster-id> mydb users \
  --schema "name:string:required,email:string:unique"

# Insert data
mgn db insert <cluster-id> mydb users \
  --data '{"name":"Alice","email":"alice@example.com"}'

# Query data
mgn db query <cluster-id> mydb users --limit 10
```

### Visualization

```bash
# Launch real-time dashboard
mgn viz
```

## Go SDK Usage

```go
package main

import (
    "context"
    "github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
)

func main() {
    // Create client
    c := client.NewClient("https://localhost:8090", "")
    ctx := context.Background()
    
    // List clusters
    clusters, _ := c.Clusters().List(ctx)
    
    // Create workspace
    spec := client.WorkspaceSpec{
        Name:  "my-workspace",
        Image: "nginx:alpine",
    }
    ws, _ := c.Workspaces(clusters[0].ID).Create(ctx, spec)
    
    // Wait for ready
    c.Workspaces(clusters[0].ID).Wait(ctx, ws.Name, 5*time.Minute)
}
```

## Automated Installation

```bash
# Full installation (microk8s + GuildNet)
cd metaguildnet/scripts/install
bash install-all.sh
```

## Configuration

### Config File

Create `~/.metaguildnet/config.yaml`:

```yaml
api:
  base_url: https://localhost:8090
  token: ""
  timeout: 30

defaults:
  cluster: my-cluster
  format: table
```

### Environment Variables

```bash
export MGN_API_URL=https://localhost:8090
export MGN_API_TOKEN=your-token
export MGN_DEFAULT_CLUSTER=production
```

## Common Tasks

### Deploy to Multiple Clusters

```bash
for cluster in cluster-1 cluster-2 cluster-3; do
  mgn workspace create $cluster \
    --name webapp \
    --image myapp:latest
done
```

### Monitor All Clusters

```bash
# Use the dashboard
mgn viz

# Or check each cluster
for cluster in $(mgn cluster list --format ids); do
  echo "Checking $cluster..."
  mgn cluster status $cluster
done
```

### Backup Configuration

```bash
# Backup cluster settings
mgn cluster get <cluster-id> -o yaml > cluster-backup.yaml

# Backup workspaces
mgn workspace list <cluster-id> -o yaml > workspaces-backup.yaml
```

## Examples

See full examples in:
- `sdk/go/examples/` - Go SDK examples
- `orchestrator/examples/` - Orchestration patterns
- `docs/examples.md` - Detailed walkthroughs

## Documentation

- [Getting Started](docs/getting-started.md)
- [Concepts](docs/concepts.md)
- [API Reference](docs/api-reference.md)
- [Examples](docs/examples.md)

## Troubleshooting

### CLI Not Found

```bash
# Make sure package is installed
cd metaguildnet/python
uv pip install -e .

# Or add to PATH
export PATH="$PATH:$HOME/.local/bin"
```

### API Connection Error

```bash
# Check if GuildNet is running
curl -k https://localhost:8090/healthz

# Verify config
mgn verify guildnet
```

### Cluster Not Reachable

```bash
# Check kubeconfig
export KUBECONFIG=$HOME/.guildnet/kubeconfig
kubectl get nodes

# Verify cluster health
mgn cluster status <cluster-id>
```

## Support

- Documentation: `metaguildnet/docs/`
- Issues: Report in GuildNet repository
- Examples: `metaguildnet/orchestrator/examples/`

