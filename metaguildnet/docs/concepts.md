# MetaGuildNet Concepts

This document explains the architecture and design patterns of MetaGuildNet and how it enhances GuildNet.

## Architecture Overview

MetaGuildNet is built as a layered enhancement on top of GuildNet:

```
┌─────────────────────────────────────────────┐
│         MetaGuildNet Layer                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │  Go SDK  │  │ Python   │  │ Scripts  │ │
│  │          │  │   CLI    │  │          │ │
│  └─────┬────┘  └────┬─────┘  └────┬─────┘ │
│        │            │             │        │
└────────┼────────────┼─────────────┼────────┘
         │            │             │
         ▼            ▼             ▼
┌─────────────────────────────────────────────┐
│         GuildNet Host App API               │
│         (internal/api/router.go)            │
└─────────────────────────────────────────────┘
         │            │             │
         ▼            ▼             ▼
┌─────────────────────────────────────────────┐
│      GuildNet Core Components               │
│  Clusters │ Workspaces │ RethinkDB │ Tsnet │
└─────────────────────────────────────────────┘
```

### Key Principles

1. **Non-invasive** - MetaGuildNet does not modify GuildNet internals
2. **Additive** - Provides convenience without restricting direct GuildNet usage
3. **Modular** - Each component can be used independently
4. **Production-ready** - All examples and patterns are deployment-ready

## Cluster Lifecycle Management

### Phases

1. **Provisioning** - Infrastructure and Kubernetes setup
2. **Bootstrap** - Initial GuildNet deployment
3. **Configuration** - Cluster settings and network setup
4. **Operation** - Day-to-day management
5. **Monitoring** - Health checks and observability
6. **Upgrade** - Version updates and migrations
7. **Decommission** - Clean shutdown and data preservation

### MetaGuildNet Support by Phase

| Phase | MetaGuildNet Tools |
|-------|-------------------|
| Provisioning | `scripts/install/01-install-microk8s.sh` |
| Bootstrap | `mgn install`, `scripts/install/04-bootstrap-cluster.sh` |
| Configuration | Go SDK `UpdateSettings()`, Python config manager |
| Operation | Go SDK, Python CLI, orchestrator examples |
| Monitoring | `mgn viz`, `mgn verify`, health API wrappers |
| Upgrade | Lifecycle examples (rolling updates, blue-green) |
| Decommission | `scripts/utils/backup-config.sh`, cleanup scripts |

## Multi-Cluster Orchestration

### Patterns

#### 1. Federation

Multiple clusters work together as a logical unit:

```yaml
# federation.yaml
federation:
  name: production
  clusters:
    - id: us-east-1
      role: primary
      weight: 60
    - id: us-west-2
      role: secondary
      weight: 40
```

**Use Cases:**
- Geographic distribution
- Load balancing
- High availability

**MetaGuildNet Support:**
- `orchestrator/examples/multi-cluster/federation.yaml`
- `orchestrator/examples/multi-cluster/load-balance.go`

#### 2. Environment Segregation

Separate clusters for different environments:

```
dev → staging → production
```

**Use Cases:**
- CI/CD pipelines
- Testing workflows
- Progressive rollout

**MetaGuildNet Support:**
- CI/CD examples in `orchestrator/examples/cicd/`
- Lifecycle management in `orchestrator/examples/lifecycle/`

#### 3. Resource Isolation

Different clusters for different workload types:

```
compute-intensive │ database │ web-services
```

**Use Cases:**
- Resource optimization
- Security boundaries
- Cost management

**MetaGuildNet Support:**
- Configuration templates in `orchestrator/templates/`
- Cluster-specific settings via Go SDK

### Coordination Strategies

#### Centralized Control

One control plane manages all clusters:

```go
// Deploy workspace to all clusters
for _, cluster := range clusters {
    _, err := c.Workspaces(cluster.ID).Create(ctx, spec)
    // Handle errors, track state
}
```

#### Declarative Configuration

Define desired state, let MetaGuildNet converge:

```yaml
workspaces:
  - name: web-frontend
    clusters: [us-east-1, us-west-2, eu-west-1]
    image: myapp:v1.2.3
```

#### Event-Driven

React to cluster events:

```go
// Watch for workspace failures, redeploy to healthy cluster
watcher := c.Workspaces(clusterID).Watch(ctx)
for event := range watcher.Events {
    if event.Type == "Failed" {
        // Find healthy cluster and redeploy
    }
}
```

## Testing and Verification Approaches

### Layered Testing

1. **Unit Tests** - Test individual SDK functions
2. **Integration Tests** - Test MetaGuildNet ↔ GuildNet API
3. **E2E Tests** - Full workflow tests
4. **Smoke Tests** - Quick verification after deployment

### Verification Strategies

#### Pre-flight Checks

Before operations, verify prerequisites:

```bash
mgn verify system      # OS, packages, permissions
mgn verify network     # Connectivity, DNS, certificates
mgn verify kubernetes  # Cluster health, resources
mgn verify guildnet    # Host App, operators, databases
```

#### Health Monitoring

Continuous health checks:

```go
// Periodic health checks
ticker := time.NewTicker(30 * time.Second)
for range ticker.C {
    health, err := c.Health().Global(ctx)
    if err != nil || !health.Healthy {
        // Alert or remediate
    }
}
```

#### Post-deployment Validation

After changes, validate success:

```bash
# Deploy workspace
mgn workspace create cluster-1 --name test-ws --image nginx

# Wait for ready
mgn workspace wait cluster-1 test-ws --timeout 5m

# Verify endpoints
mgn workspace test cluster-1 test-ws --http-check /
```

## SDK Design Patterns

### Error Handling

MetaGuildNet SDK provides rich error context:

```go
ws, err := c.Workspaces(clusterID).Get(ctx, name)
if err != nil {
    if errors.Is(err, client.ErrNotFound) {
        // Handle not found
    } else if errors.Is(err, client.ErrUnauthorized) {
        // Handle auth error
    } else {
        // Generic error handling
    }
}
```

### Retry Logic

Built-in retries for transient failures:

```go
// Automatic retries with exponential backoff
c := client.NewClient(baseURL, token, 
    client.WithMaxRetries(3),
    client.WithRetryBackoff(time.Second))
```

### Context Propagation

Proper context handling for timeouts and cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()

ws, err := c.Workspaces(clusterID).Create(ctx, spec)
```

### Resource Cleanup

Ensure resources are properly released:

```go
defer func() {
    if err := c.Workspaces(clusterID).Delete(ctx, ws.Name); err != nil {
        log.Printf("cleanup failed: %v", err)
    }
}()
```

## Configuration Management

### Configuration Hierarchy

Settings are resolved in this order (last wins):

1. Built-in defaults
2. Configuration files (`~/.metaguildnet/config.yaml`)
3. Environment variables (`MGN_*`)
4. Command-line flags
5. API calls

### Configuration Files

```yaml
# ~/.metaguildnet/config.yaml
api:
  base_url: https://localhost:8090
  token: ""
  timeout: 30s

clusters:
  default: cluster-1
  
preferences:
  log_level: info
  color: auto
```

### Environment Variables

```bash
export MGN_API_URL=https://guildnet.local:8090
export MGN_API_TOKEN=secret
export MGN_DEFAULT_CLUSTER=production
```

## Integration Points

MetaGuildNet integrates with:

1. **GuildNet Host App** - Primary API
2. **Kubernetes API** - Direct cluster access when needed
3. **RethinkDB** - Database operations
4. **Tailscale/Headscale** - Network verification
5. **CI/CD Systems** - Automated workflows

## Best Practices

### 1. Use Configuration Templates

Start with templates in `orchestrator/templates/` and customize:

```bash
cp orchestrator/templates/cluster-production.yaml my-cluster.yaml
# Edit my-cluster.yaml
mgn cluster apply -f my-cluster.yaml
```

### 2. Verify Before Changes

Always verify before making changes:

```bash
mgn verify all
mgn cluster status <id>
```

### 3. Use Namespaces

Organize workspaces with labels:

```go
spec.Labels = map[string]string{
    "env": "production",
    "app": "frontend",
    "version": "v1.2.3",
}
```

### 4. Monitor Operations

Use the dashboard for long-running operations:

```bash
# Terminal 1: Start operation
mgn workspace create cluster-1 --name big-deployment --image large:latest

# Terminal 2: Monitor
mgn viz
```

### 5. Automate Verification

Include verification in your workflows:

```bash
# In CI/CD
mgn verify all || exit 1
mgn workspace create ...
mgn workspace wait ...
mgn workspace test ...
```

## Next Steps

- See [examples](examples.md) for detailed walkthroughs
- Review [API reference](api-reference.md) for complete SDK documentation
- Explore [orchestrator examples](../orchestrator/examples/) for production patterns

