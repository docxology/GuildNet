# Getting Started with MetaGuildNet

This guide walks you through installing and using MetaGuildNet with an existing GuildNet deployment.

## Prerequisites

Before using MetaGuildNet, ensure you have:

1. **GuildNet installed** - Follow [DEPLOYMENT.md](../../DEPLOYMENT.md) if needed
2. **Python 3.11+** with uv installed (`pip install uv`)
3. **Go 1.22+** for SDK examples
4. **kubectl** configured with access to your Kubernetes cluster
5. **GuildNet Host App running** (typically at `https://localhost:8090`)

## Installation

### Python CLI

```bash
cd metaguildnet/python
uv pip install -e .

# Verify installation
mgn --version
```

### Go SDK

The Go SDK is importable directly:

```go
import "github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
```

No separate installation needed - it uses the same module as GuildNet.

## First Cluster Setup

### 1. Verify Your Environment

```bash
# Run comprehensive verification
mgn verify all

# Or individual checks
mgn verify system
mgn verify network
mgn verify kubernetes
mgn verify guildnet
```

### 2. Check Cluster Status

Using the CLI:

```bash
mgn cluster list
mgn cluster status <cluster-id>
```

Using the Go SDK:

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
)

func main() {
    c := client.NewClient("https://localhost:8090", "")
    ctx := context.Background()
    
    clusters, err := c.Clusters().List(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, cluster := range clusters {
        fmt.Printf("Cluster: %s (%s)\n", cluster.Name, cluster.ID)
        
        health, _ := c.Health().Cluster(ctx, cluster.ID)
        fmt.Printf("  K8s Reachable: %v\n", health.K8sReachable)
        fmt.Printf("  Kubeconfig Valid: %v\n", health.KubeconfigValid)
    }
}
```

### 3. Create Your First Workspace

Using the CLI:

```bash
mgn workspace create <cluster-id> \
  --name my-first-workspace \
  --image codercom/code-server:latest \
  --env PASSWORD=securepass
```

Using the Go SDK:

```go
spec := client.WorkspaceSpec{
    Image: "codercom/code-server:latest",
    Env: []client.EnvVar{
        {Name: "PASSWORD", Value: "securepass"},
    },
}

ws, err := c.Workspaces(clusterID).Create(ctx, spec)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Workspace created: %s\n", ws.Name)
```

### 4. Monitor Workspace Status

```bash
# Watch workspace until ready
mgn workspace wait <cluster-id> my-first-workspace

# View logs
mgn workspace logs <cluster-id> my-first-workspace

# Stream logs in real-time
mgn workspace logs <cluster-id> my-first-workspace --follow
```

## Basic Orchestration Patterns

### Deploy to Multiple Clusters

```bash
# List all clusters
mgn cluster list

# Deploy the same workspace to all clusters
for cluster in $(mgn cluster list --format ids); do
  mgn workspace create $cluster --name shared-workspace --image nginx:alpine
done
```

### Health Monitoring Dashboard

```bash
# Launch real-time dashboard
mgn viz
```

This opens an interactive terminal dashboard showing:
- Cluster status
- Workspace health
- Resource utilization
- Real-time logs

## Common Troubleshooting

### GuildNet Host App Not Reachable

```bash
# Check if Host App is running
curl -k https://localhost:8090/healthz

# Check Host App logs
journalctl -u guildnet-hostapp -f
```

### Kubeconfig Issues

```bash
# Verify kubectl works
kubectl get nodes

# Check GuildNet kubeconfig
ls -la ~/.guildnet/kubeconfig

# Re-verify cluster
mgn verify kubernetes
```

### Workspace Not Starting

```bash
# Check workspace status
mgn workspace describe <cluster-id> <workspace-name>

# View logs
mgn workspace logs <cluster-id> <workspace-name>

# Check cluster-level issues
mgn cluster diagnose <cluster-id>
```

## Next Steps

- Explore [orchestration examples](examples.md)
- Learn about [multi-cluster patterns](concepts.md#multi-cluster-orchestration)
- Set up [CI/CD integration](../orchestrator/examples/cicd/README.md)
- Review [API reference](api-reference.md)

