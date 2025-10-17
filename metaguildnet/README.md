# MetaGuildNet

MetaGuildNet is a comprehensive utilities and orchestration layer for GuildNet. It provides convenience SDKs, automation scripts, orchestration examples, and verification tools to simplify the deployment, management, and operation of GuildNet clusters.

## Philosophy

MetaGuildNet acts as a "rider" on top of GuildNet - it does not replace or fork GuildNet, but instead provides higher-level abstractions and utilities that make it easier to:

- **Configure** - Manage complex multi-cluster configurations
- **Install** - Automated, reproducible installation workflows
- **Use** - Convenient SDKs and CLIs for common operations
- **Verify** - Comprehensive verification and health checking
- **Visualize** - Real-time dashboards and status monitoring
- **Orchestrate** - Multi-cluster deployment patterns and lifecycle management

## Prerequisites

MetaGuildNet requires a working GuildNet installation. If you don't have GuildNet installed yet, follow the [GuildNet installation guide](../DEPLOYMENT.md) first.

## Quick Start

### Install MetaGuildNet CLI (Python)

```bash
cd metaguildnet/python
uv pip install -e .
```

### Verify Your GuildNet Installation

```bash
mgn verify all
```

### Use the Go SDK

```go
import "github.com/docxology/GuildNet/metaguildnet/sdk/go/client"

c := client.NewClient("https://localhost:8090", "")
clusters, err := c.Clusters().List()
```

### Run an Orchestrator Example

```bash
cd metaguildnet/orchestrator/examples/multi-cluster
./deploy-federated.sh
```

## Documentation

- [Getting Started](docs/getting-started.md) - Installation and first steps
- [Concepts](docs/concepts.md) - Architecture and patterns
- [Examples](docs/examples.md) - Detailed walkthroughs
- [API Reference](docs/api-reference.md) - SDK and CLI documentation

## Components

### Go SDK (`sdk/go/`)

Convenience wrappers around the GuildNet Host App API:
- Cluster management
- Workspace operations
- Database interactions
- Health monitoring
- Testing utilities

### Python CLI (`python/`)

Command-line interface and automation tools:
- `mgn cluster` - Manage clusters
- `mgn workspace` - Workspace operations
- `mgn install` - Automated installation
- `mgn verify` - Verification suite
- `mgn viz` - Real-time dashboard

### Orchestrator Examples (`orchestrator/`)

Production-ready patterns:
- Multi-cluster federation
- Lifecycle management (rolling updates, blue-green, canary)
- CI/CD integration examples
- Configuration templates

### Scripts (`scripts/`)

Comprehensive automation:
- Installation scripts
- Verification scripts
- Utility scripts (logging, debugging, backup)

### Tests (`tests/`)

Integration and end-to-end tests:
- Go SDK tests
- Python CLI tests
- Orchestrator example tests
- Full workflow tests

## Contributing

MetaGuildNet follows GuildNet's development principles:
- Modular, composable components
- Production-first (no dev/local modes)
- Sensible defaults with customization via environment variables
- Well-documented, clearly-commented code

## License

Same as GuildNet - see [LICENSE](../LICENSE)

