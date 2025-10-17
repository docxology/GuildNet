# MetaGuildNet Runner (run.py)

**Programmatic execution of MetaGuildNet workflows with configuration support.**

## Overview

The `run.py` script provides a programmatic interface to execute MetaGuildNet workflows. It supports:

- **Configuration-driven execution** via JSON config files
- **Individual workflow steps** (setup, verify, test, examples, etc.)
- **Colored logging** with different verbosity levels
- **Error handling** and graceful failure recovery
- **Environment variable support** for customization

## Quick Start

### Basic Usage

```bash
# Run full workflow with default config
python3 run.py

# Or use the wrapper script
./run.sh

# Run with custom config
python3 run.py --config dev-config.json

# Show configuration without running
python3 run.py --dry-run
```

### Available Workflows

```bash
# Run specific workflows
python3 run.py --workflow setup     # Setup only
python3 run.py --workflow verify    # Verification only
python3 run.py --workflow test      # Testing only
python3 run.py --workflow example   # Examples only
python3 run.py --workflow diagnose  # Diagnostics only
python3 run.py --workflow cleanup   # Cleanup only

# Run full workflow (default)
python3 run.py --workflow full
```

## Configuration

### Default Configuration (`config.json`)

```json
{
  "meta_setup": {
    "enabled": true,
    "mode": "auto",
    "verify_timeout": 300,
    "auto_approve_routes": true,
    "log_level": "info"
  },
  "verification": {
    "enabled": true,
    "layers": ["network", "cluster", "database", "application"],
    "output_format": "text",
    "timeout": 300
  },
  "testing": {
    "enabled": true,
    "run_integration_tests": true,
    "run_e2e_tests": true,
    "test_timeout": 600
  },
  "examples": {
    "enabled": true,
    "create_workspace": true,
    "workspace_image": "codercom/code-server:latest",
    "workspace_name": "metaguildnet-demo",
    "multi_user_setup": false
  },
  "diagnostics": {
    "enabled": true,
    "export_diagnostics": false,
    "diagnostic_timeout": 60
  },
  "cleanup": {
    "enabled": false,
    "remove_workspaces": true,
    "stop_services": false
  },
  "logging": {
    "level": "info",
    "format": "colored",
    "timestamp": true,
    "file": null
  }
}
```

### Configuration Options

#### Setup Options (`meta_setup`)
- `enabled`: Enable/disable setup workflow
- `mode`: Setup mode (`auto`, `interactive`, `minimal`)
- `verify_timeout`: Verification timeout in seconds
- `auto_approve_routes`: Auto-approve Tailscale routes
- `log_level`: Log level for setup (`debug`, `info`, `warning`, `error`)

#### Verification Options (`verification`)
- `enabled`: Enable/disable verification
- `layers`: Layers to verify (`network`, `cluster`, `database`, `application`)
- `output_format`: Output format (`text`, `json`)
- `timeout`: Verification timeout in seconds

#### Testing Options (`testing`)
- `enabled`: Enable/disable testing
- `run_integration_tests`: Run integration tests
- `run_e2e_tests`: Run end-to-end tests
- `test_timeout`: Test timeout in seconds

#### Examples Options (`examples`)
- `enabled`: Enable/disable examples
- `create_workspace`: Create demo workspace
- `workspace_image`: Docker image for workspace
- `workspace_name`: Name for demo workspace
- `multi_user_setup`: Run multi-user setup example

#### Diagnostics Options (`diagnostics`)
- `enabled`: Enable/disable diagnostics
- `export_diagnostics`: Export diagnostic bundle
- `diagnostic_timeout`: Diagnostic timeout in seconds

#### Cleanup Options (`cleanup`)
- `enabled`: Enable/disable cleanup
- `remove_workspaces`: Remove workspaces
- `stop_services`: Stop services

#### Logging Options (`logging`)
- `level`: Log level (`debug`, `info`, `warning`, `error`)
- `format`: Output format (`colored`, `plain`)
- `timestamp`: Show timestamps
- `file`: Log file path (null for console only)

## Command Line Options

```bash
usage: run.py [-h] [--config CONFIG] [--dry-run]
              [--workflow {full,setup,verify,test,example,diagnose,cleanup}]
              [--log-level {debug,info,warning,error}]

MetaGuildNet Runner - Programmatic GuildNet workflow execution

options:
  -h, --help            show this help message and exit
  --config CONFIG, -c CONFIG
                        Configuration file path (default: config.json)
  --dry-run             Show configuration and exit without running
  --workflow {full,setup,verify,test,example,diagnose,cleanup}
                        Workflow to run (default: full)
  --log-level {debug,info,warning,error}
                        Log level (default: info)
```

## Examples

### Development Workflow

```bash
# 1. Check configuration
python3 run.py --dry-run

# 2. Run verification only
python3 run.py --workflow verify --log-level debug

# 3. Run tests only
python3 run.py --workflow test

# 4. Create example workspace
python3 run.py --workflow example
```

### Custom Configuration

```bash
# Create custom config for development
cp config.json dev-config.json

# Edit dev-config.json
# Set testing.enabled = true
# Set examples.enabled = false
# Set logging.level = debug

# Run with custom config
python3 run.py --config dev-config.json --workflow full
```

### CI/CD Integration

```bash
#!/bin/bash
# ci-test.sh

# Install dependencies
pip install -r requirements.txt

# Run verification
python3 MetaGuildNet/run.py --workflow verify --log-level info

# Run tests
python3 MetaGuildNet/run.py --workflow test --log-level info

# Export diagnostics on failure
if [ $? -ne 0 ]; then
    python3 MetaGuildNet/run.py --workflow diagnose --config ci-config.json
fi
```

### Production Deployment

```bash
# Production config
cat > prod-config.json << EOF
{
  "meta_setup": {
    "enabled": true,
    "mode": "auto",
    "auto_approve_routes": true
  },
  "verification": {
    "enabled": true,
    "timeout": 600
  },
  "testing": {
    "enabled": true,
    "run_integration_tests": true,
    "run_e2e_tests": true
  },
  "examples": {
    "enabled": false
  },
  "diagnostics": {
    "enabled": true,
    "export_diagnostics": true
  },
  "cleanup": {
    "enabled": true,
    "remove_workspaces": true
  }
}
EOF

# Deploy
python3 run.py --config prod-config.json --workflow full
```

## Output Formats

### Text Output (Default)

```
╔════════════════════════════════════════╗
║ MetaGuildNet Setup                     ║
╚════════════════════════════════════════╝

[15:23:45] ✓ Prerequisites check passed
[15:23:46] ✓ Network layer configured
[15:23:47] ✓ Cluster layer deployed
[15:23:48] ✓ Application layer started
[15:23:49] ✓ Setup completed successfully
```

### JSON Output (for automation)

```bash
python3 run.py --workflow verify --config automation.json
```

```json
{
  "timestamp": "2025-10-13T15:23:49Z",
  "overall_status": "healthy",
  "duration_seconds": 4,
  "layers": [
    {
      "name": "network",
      "status": "healthy",
      "duration_seconds": 1
    }
  ]
}
```

## Error Handling

The script provides comprehensive error handling:

- **Graceful failures**: Continues with other steps when possible
- **Detailed error messages**: Shows what went wrong and suggestions
- **Exit codes**: 0 for success, 1 for failure, 130 for interruption
- **Timeout handling**: Respects timeout settings for long-running operations

## Logging

### Log Levels

- `debug`: Detailed internal information
- `info`: General progress and results
- `warning`: Non-critical issues
- `error`: Critical failures

### Log Format

```
[15:23:45] ✓ Prerequisites check passed
[15:23:46] ⚠ Network layer issue detected
[15:23:47] ✗ Setup failed: timeout exceeded
```

### Custom Log Files

```json
{
  "logging": {
    "file": "/var/log/metaguildnet.log"
  }
}
```

## Integration Examples

### Python API Usage

```python
from pathlib import Path
from run import MetaGuildNetRunner

# Create runner with custom config
runner = MetaGuildNetRunner("custom-config.json")

# Run specific workflows
success = runner.run_verification()
if success:
    print("Verification passed!")

# Run full workflow
success = runner.run_full_workflow()
```

### Shell Script Integration

```bash
#!/bin/bash
# deploy.sh

# Source environment
set -a
source .env
set +a

# Run MetaGuildNet
cd MetaGuildNet
python3 run.py --config prod-config.json --workflow full

# Check result
if [ $? -eq 0 ]; then
    echo "Deployment successful"
else
    echo "Deployment failed"
    exit 1
fi
```

## Troubleshooting

### Common Issues

1. **"No rule to make target"**
   - Ensure you're running from the correct directory
   - Check that Makefile targets exist

2. **"Permission denied"**
   - Make scripts executable: `chmod +x run.py run.sh`

3. **"Module not found"**
   - Ensure Python 3 is installed: `python3 --version`

4. **Configuration errors**
   - Validate JSON: `python3 -m json.tool config.json`

### Debug Mode

```bash
# Enable debug logging
python3 run.py --log-level debug --workflow verify

# Show full stack traces
PYTHONUNBUFFERED=1 python3 run.py --workflow setup 2>&1 | tee debug.log
```

## Advanced Usage

### Environment Variables

The script respects environment variables set in the shell:

```bash
export METAGN_SETUP_MODE=interactive
export METAGN_LOG_LEVEL=debug
python3 run.py
```

### Custom Workflows

Create workflow-specific configs:

```bash
# Quick verification
python3 run.py --workflow verify --config quick-check.json

# Full deployment
python3 run.py --workflow full --config full-deploy.json
```

### Batch Operations

```bash
#!/bin/bash
# batch-deploy.sh

declare -a configs=("dev.json" "staging.json" "prod.json")

for config in "${configs[@]}"; do
    echo "Deploying with $config..."
    python3 run.py --config "$config" --workflow full

    if [ $? -ne 0 ]; then
        echo "Deployment failed for $config"
        exit 1
    fi
done

echo "All deployments successful!"
```

## Performance

### Typical Execution Times

- **Setup**: 5-15 minutes (depending on network/cluster setup)
- **Verification**: 30-60 seconds
- **Testing**: 2-10 minutes (depending on test scope)
- **Examples**: 1-5 minutes

### Optimization Tips

1. **Parallel execution**: Some steps can run concurrently
2. **Timeout tuning**: Adjust timeouts for your environment
3. **Selective workflows**: Run only needed steps
4. **Caching**: Reuse previous deployments when possible

## Security Considerations

- Configuration files may contain sensitive information
- Use appropriate file permissions: `chmod 600 config.json`
- Avoid logging sensitive data in custom configs
- Validate configurations from trusted sources only

## Contributing

To extend the runner:

1. Add new workflow methods to `MetaGuildNetRunner` class
2. Update configuration schema in `_get_default_config()`
3. Add command-line options in `main()` function
4. Document new features in this README

## Support

- **Documentation**: See MetaGuildNet/docs/ for detailed guides
- **Issues**: Report bugs in the GitHub repository
- **Discussions**: Use GitHub Discussions for questions

---

**Example Workflow**:

```bash
# 1. Check current status
python3 run.py --workflow diagnose

# 2. Run verification
python3 run.py --workflow verify

# 3. Run tests if verification passes
python3 run.py --workflow test

# 4. Create demo workspace
python3 run.py --workflow example
```

This provides a complete, automated workflow for MetaGuildNet operations! 🚀
