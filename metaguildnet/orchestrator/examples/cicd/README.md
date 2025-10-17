# CI/CD Integration Examples

This directory contains examples for integrating GuildNet deployments into CI/CD pipelines.

## Available Examples

- `github-actions.yaml` - GitHub Actions workflow
- `gitlab-ci.yaml` - GitLab CI pipeline  
- `jenkins/Jenkinsfile` - Jenkins pipeline

## Common Pattern

All CI/CD integrations follow this pattern:

1. **Build** - Build and push Docker image
2. **Deploy** - Deploy to staging/production clusters
3. **Verify** - Run health checks
4. **Rollback** - Automatic rollback on failure

## GitHub Actions

```yaml
# .github/workflows/deploy.yml
# See github-actions.yaml for complete example
```

Features:
- Matrix deployment to multiple clusters
- Automatic rollback
- Slack notifications
- Environment protection

## GitLab CI

```yaml
# .gitlab-ci.yml
# See gitlab-ci.yaml for complete example
```

Features:
- Multi-stage pipeline
- Manual approval gates
- Environment-specific secrets
- Deployment tracking

## Jenkins

```groovy
// Jenkinsfile
// See jenkins/Jenkinsfile for complete example
```

Features:
- Declarative pipeline
- Parallel deployment
- Input approval
- Post-deployment testing

## Setup

### 1. Install MetaGuildNet CLI

```bash
pip install metaguildnet
```

### 2. Configure Secrets

Set these secrets in your CI/CD system:

- `GUILDNET_API_URL` - GuildNet Host App URL
- `GUILDNET_API_TOKEN` - API authentication token
- `DOCKER_REGISTRY` - Docker registry URL
- `DOCKER_USERNAME` - Registry username
- `DOCKER_PASSWORD` - Registry password

### 3. Customize Workflow

Copy the appropriate example and customize:

- Cluster IDs
- Image names
- Deployment strategy
- Health check endpoints

## Best Practices

1. **Use Staging** - Always deploy to staging first
2. **Health Checks** - Verify deployments with health checks
3. **Rollback Plan** - Have automatic rollback on failure
4. **Notifications** - Alert team on deployment events
5. **Audit Trail** - Keep deployment logs and metadata

## Example Workflow

```bash
# 1. Build image
docker build -t myapp:$VERSION .

# 2. Push to registry
docker push myapp:$VERSION

# 3. Deploy to staging
mgn workspace create staging-cluster \
  --name myapp \
  --image myapp:$VERSION \
  --update-if-exists

# 4. Run tests
mgn workspace wait staging-cluster myapp
mgn workspace test staging-cluster myapp --http-check /health

# 5. Deploy to production
for cluster in prod-us prod-eu; do
  mgn workspace create $cluster \
    --name myapp \
    --image myapp:$VERSION \
    --update-if-exists
done

# 6. Verify
for cluster in prod-us prod-eu; do
  mgn workspace wait $cluster myapp --timeout 10m
  mgn workspace test $cluster myapp --http-check /health
done
```

## Troubleshooting

### Deployment Fails

```bash
# Check logs
mgn workspace logs <cluster-id> <workspace-name>

# Check cluster health
mgn cluster status <cluster-id>

# Manual rollback
mgn workspace delete <cluster-id> <workspace-name>
mgn workspace create <cluster-id> --name <workspace-name> --image <previous-image>
```

### Authentication Issues

```bash
# Verify credentials
mgn cluster list

# Check API token
curl -H "Authorization: Bearer $GUILDNET_API_TOKEN" \
  https://$GUILDNET_API_URL/api/health
```

## See Also

- [../lifecycle/](../lifecycle/) - Deployment patterns
- [../../templates/](../../templates/) - Configuration templates
- [../../../docs/examples.md](../../../docs/examples.md) - More examples

