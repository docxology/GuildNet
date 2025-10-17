# MetaGuildNet Python CLI

Command-line interface and Python SDK for GuildNet management.

## Installation

Using uv (recommended):

```bash
uv pip install -e .
```

Using pip:

```bash
pip install -e .
```

## Quick Start

```bash
# Verify your GuildNet installation
mgn verify all

# List clusters
mgn cluster list

# Create a workspace
mgn workspace create <cluster-id> --name my-workspace --image nginx:alpine

# Monitor status
mgn viz
```

## Commands

### Cluster Management

```bash
mgn cluster list                    # List all clusters
mgn cluster get <id>                # Get cluster details
mgn cluster status <id>             # Check cluster health
mgn cluster update <id> --setting key=value  # Update settings
```

### Workspace Operations

```bash
mgn workspace list <cluster-id>     # List workspaces
mgn workspace create <cluster-id> --name <name> --image <image>
mgn workspace get <cluster-id> <name>
mgn workspace delete <cluster-id> <name>
mgn workspace logs <cluster-id> <name> [--follow]
mgn workspace wait <cluster-id> <name> [--timeout 5m]
```

### Database Operations

```bash
mgn db list <cluster-id>            # List databases
mgn db create <cluster-id> <name>   # Create database
mgn db table create <cluster-id> <db-id> <table> --schema <spec>
mgn db insert <cluster-id> <db-id> <table> --data <json>
mgn db query <cluster-id> <db-id> <table> [--limit 100]
```

### Installation

```bash
mgn install [--type local] [--cluster-name dev]
```

### Verification

```bash
mgn verify system      # System prerequisites
mgn verify network     # Network connectivity
mgn verify kubernetes  # Kubernetes cluster
mgn verify guildnet    # GuildNet installation
mgn verify all         # All checks
```

### Visualization

```bash
mgn viz  # Real-time dashboard
```

## Configuration

Configuration file: `~/.metaguildnet/config.yaml`

```yaml
api:
  base_url: https://localhost:8090
  token: ""
  timeout: 30

defaults:
  cluster: my-cluster
  format: table

logging:
  level: info
```

Environment variables:

```bash
export MGN_API_URL=https://localhost:8090
export MGN_API_TOKEN=secret
export MGN_DEFAULT_CLUSTER=production
```

## Development

Install development dependencies:

```bash
uv pip install -e ".[dev]"
```

Run tests:

```bash
pytest
```

Lint:

```bash
ruff check .
mypy src/metaguildnet
```

Format:

```bash
ruff format .
```

