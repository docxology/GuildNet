# Lifecycle Management Examples

This directory contains examples for managing workspace lifecycle across deployments.

## Deployment Patterns

### Rolling Update

Gradually update workspaces with zero downtime:

```bash
./rolling-update.sh <cluster-id> <workspace-name> <new-image>
```

### Blue-Green Deployment

Deploy new version alongside old, then switch traffic:

```bash
go run blue-green.go --cluster <id> --workspace <name> --new-image <image>
```

### Canary Deployment

Deploy to small subset of instances first:

```bash
./canary.sh <cluster-id> <workspace-name> <new-image> <canary-percentage>
```

## Patterns Explained

### Rolling Update
- Create new version
- Wait for health checks
- Delete old version
- Zero downtime

### Blue-Green
- Run old (blue) and new (green) in parallel
- Test green thoroughly
- Switch traffic to green
- Remove blue

### Canary
- Deploy to small percentage
- Monitor metrics
- Gradually increase traffic
- Rollback if issues detected

## Safety Features

All examples include:
- Health checks before switching
- Automatic rollback on failure
- State preservation
- Audit logging

