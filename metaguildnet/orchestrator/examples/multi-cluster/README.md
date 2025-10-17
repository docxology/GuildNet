# Multi-Cluster Orchestration Examples

This directory contains examples for managing workspaces across multiple GuildNet clusters.

## Files

- `federation.yaml` - Multi-cluster configuration example
- `deploy-federated.sh` - Deploy workspaces to multiple clusters
- `load-balance.go` - Load balancing example using Go SDK

## Quick Start

### 1. Configure Federation

Edit `federation.yaml` to define your cluster federation:

```yaml
federation:
  name: production
  clusters:
    - id: cluster-us-east
      role: primary
      weight: 60
    - id: cluster-us-west
      role: secondary
      weight: 40
```

### 2. Deploy Workspace

Deploy a workspace to all federated clusters:

```bash
./deploy-federated.sh federation.yaml workspace.yaml
```

### 3. Load Balancing

Run the load balancing example:

```bash
go run load-balance.go
```

## Use Cases

- **Geographic Distribution**: Deploy workspaces closer to users
- **High Availability**: Redundancy across clusters
- **Load Distribution**: Balance workload across infrastructure

## Patterns

### Active-Active

Deploy identical workspaces to all clusters with traffic distribution:

```bash
for cluster in us-east us-west eu-west; do
  mgn workspace create $cluster --name webapp --image myapp:latest
done
```

### Active-Passive

Deploy to primary cluster, failover to secondary on failure:

```bash
# Deploy to primary
mgn workspace create primary --name webapp --image myapp:latest

# Monitor and failover if needed
if ! mgn workspace test primary webapp; then
  mgn workspace create secondary --name webapp --image myapp:latest
fi
```

### Canary Deployment

Deploy new version to subset of clusters:

```bash
# Deploy v2 to canary cluster
mgn workspace create canary --name webapp --image myapp:v2

# Test
if mgn workspace test canary webapp; then
  # Roll out to all clusters
  for cluster in prod-1 prod-2 prod-3; do
    mgn workspace create $cluster --name webapp --image myapp:v2 --update
  done
fi
```

## See Also

- [../lifecycle/](../lifecycle/) - Deployment lifecycle patterns
- [../../templates/](../../templates/) - Configuration templates
- [../../../docs/concepts.md](../../../docs/concepts.md) - Multi-cluster concepts

