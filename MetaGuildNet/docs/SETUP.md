# MetaGuildNet Setup Guide

**Comprehensive setup procedures with troubleshooting**

## Overview

This guide covers all setup scenarios from single-machine development to production multi-node deployments.

## Prerequisites

### System Requirements

**Minimum** (Development):
- OS: Linux (kernel 5.10+), macOS 12+
- CPU: 4 cores
- RAM: 8 GB
- Disk: 50 GB free
- Network: Stable internet connection

**Recommended** (Production):
- OS: Ubuntu 22.04 LTS or similar
- CPU: 8+ cores
- RAM: 16+ GB
- Disk: 100+ GB SSD
- Network: 100+ Mbps with static IP

### Required Software

The setup wizard will check and help install these:

```bash
# Core tools
- bash >= 4.0
- make >= 4.0
- docker >= 20.10
- go >= 1.22
- node >= 18
- kubectl >= 1.30

# Optional (for full stack)
- talosctl (for Talos cluster management)
- helm (for K8s package management)
```

### Check Prerequisites

```bash
# Run the prerequisite checker
make -C MetaGuildNet check-prereqs

# Output shows what's installed and what's missing
✓ bash 5.1.16
✓ make 4.3
✓ docker 24.0.5
✓ go 1.22.1
✓ node 18.16.0
✓ kubectl 1.30.1
⚠ talosctl not found (optional)
✓ helm 3.12.0

Prerequisites: 7/8 required, 8/9 total
Status: Ready for setup
```

## Setup Modes

### 1. Automatic Mode (Recommended)

Full hands-off setup with sensible defaults:

```bash
make meta-setup
```

This will:
1. Check prerequisites (install if missing)
2. Setup Headscale (Docker container on LAN IP)
3. Setup Tailscale router (advertise cluster routes)
4. Deploy Talos cluster (single-node dev)
5. Install addons (MetalLB, CRDs, RethinkDB)
6. Deploy Host App with embedded operator
7. Run verification suite
8. Display access URLs

**Duration**: ~10 minutes on modern hardware

### 2. Interactive Mode

Step-by-step with choices:

```bash
METAGN_SETUP_MODE=interactive make meta-setup
```

You'll be prompted at each major decision point:

```mermaid
flowchart TD
    Start([MetaGuildNet Setup Wizard<br/>METAGN_SETUP_MODE=interactive make meta-setup]) --> Step1

    subgraph "Step 1/7: Network Layer"
        Step1{Choose Network Setup<br/>MetaGuildNet/scripts/setup/network_wizard.sh} --> Option1[1) Create new Headscale server<br/>Docker container on LAN IP]
        Step1 --> Option2[2) Use existing Headscale server<br/>External Headscale instance]
        Step1 --> Option3[3) Skip network setup<br/>Already configured]

        Option1 --> Validation1[Validate prerequisites<br/>Docker, certificates]
        Option2 --> Validation2[Validate connection<br/>TS_LOGIN_SERVER reachable]
        Option3 --> Validation3[Verify existing setup<br/>tailscale status, routes]
    end

    Validation1 --> Step2
    Validation2 --> Step2
    Validation3 --> Step2

    subgraph "Step 2/7: Cluster Layer"
        Step2{Choose Cluster Setup<br/>MetaGuildNet/scripts/setup/cluster_wizard.sh} --> K8s1[1) Deploy Talos cluster<br/>Single/multi-node]
        Step2 --> K8s2[2) Use existing Kubernetes<br/>External cluster]
    end

    style Start fill:#e8f5e8
    style Step1 fill:#fff3e0
    style Step2 fill:#e3f2fd
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for the complete setup orchestration flow.

### 3. Minimal Mode

Core components only (no cluster):

```bash
METAGN_SETUP_MODE=minimal make meta-setup
```

Installs:
- Headscale
- Tailscale router
- Host App (proxy mode only)

Good for testing network layer independently.

### 4. Custom Mode

Maximum control:

```bash
# Setup individual layers
make meta-setup-network
make meta-setup-cluster
make meta-setup-application

# Or cherry-pick components
make headscale-up
make router-up
make setup-talos
make deploy-k8s-addons
make run
```

## Step-by-Step Setup (Manual)

### Step 1: Clone and Prepare

```bash
# Clone the repository
git clone https://github.com/your-org/GuildNet.git
cd GuildNet

# Install UI dependencies
cd ui && npm ci && cd ..

# Generate TLS certificates
./scripts/generate-server-cert.sh -f
```

### Step 2: Network Layer

#### 2a. Headscale

```bash
# Start Headscale bound to LAN IP
make headscale-up

# Check status
make headscale-status

# Expected output:
# Container: guildnet-headscale (running)
# Port: 8080/tcp -> 0.0.0.0:8080
# Config: /etc/headscale/config.yaml
# URL: http://192.168.1.100:8080
```

#### 2b. Bootstrap Headscale

```bash
# Create user + preauth key
make headscale-bootstrap

# This creates/updates .env with:
# TS_LOGIN_SERVER=http://192.168.1.100:8080
# TS_AUTHKEY=<generated-key>
```

#### 2c. Tailscale Router

```bash
# Install Tailscale
make router-install

# Start router (advertises cluster routes)
make router-up

# Approve routes in Headscale
make headscale-approve-routes

# Verify
make router-status

# Expected output:
# Tailscale status:
# - Hostname: guildnet-router
# - IP: 100.64.0.1
# - Routes: 10.96.0.0/12, 10.244.0.0/16 (advertised, approved)
```

### Step 3: Cluster Layer

#### 3a. Talos Cluster

```bash
# Deploy Talos (single-node dev cluster)
make setup-talos

# This runs:
# - Talos config generation
# - Cluster bootstrap
# - Kubeconfig export to ~/.guildnet/kubeconfig
# - Wait for API readiness

# Verify
export KUBECONFIG=~/.guildnet/kubeconfig
kubectl get nodes

# Expected output:
# NAME     STATUS   ROLES           AGE   VERSION
# talos-1  Ready    control-plane   2m    v1.30.1
```

#### 3b. Kubernetes Add-ons

```bash
# Install MetalLB, CRDs, RethinkDB
make deploy-k8s-addons

# This installs:
# - MetalLB (LoadBalancer for Services)
# - GuildNet CRDs (Workspace, Capability)
# - RethinkDB (persistent database)
# - imagePullSecret (for private registry)

# Verify MetalLB
kubectl get pods -n metallb-system

# Verify RethinkDB
kubectl get pods -l app=rethinkdb

# Verify CRDs
kubectl get crds | grep guildnet.io
```

### Step 4: Application Layer

#### 4a. Build Backend

```bash
# Build Go binary
make build-backend

# Output: bin/hostapp
```

#### 4b. Build UI

```bash
# Build Vite app
make build-ui

# Output: ui/dist/
```

#### 4c. Run Host App

```bash
# Run with embedded operator
make run

# Access:
# - Local: https://127.0.0.1:8080
# - Tailnet: https://<ts-hostname>:443
```

### Step 5: Verification

```bash
# Run comprehensive verification
make meta-verify

# Output shows health of all layers:
# [NETWORK] ✓ Healthy
# [CLUSTER] ✓ Healthy
# [DATABASE] ✓ Healthy
# [APPLICATION] ✓ Healthy
```

## Environment Configuration

### Core Variables

```bash
# Network
TS_LOGIN_SERVER=http://192.168.1.100:8080  # Headscale URL
TS_AUTHKEY=<key>                             # Pre-auth key
TS_ROUTES=10.96.0.0/12,10.244.0.0/16        # Advertised routes

# Cluster
KUBECONFIG=~/.guildnet/kubeconfig            # Kube config path
CP_NODES=192.168.1.101                       # Control plane nodes
WK_NODES=192.168.1.102,192.168.1.103        # Worker nodes (optional)
K8S_NAMESPACE=default                        # Default namespace

# Application
LISTEN_LOCAL=127.0.0.1:8080                  # Local bind address
FRONTEND_ORIGIN=https://127.0.0.1:8080      # CORS origin
HOSTAPP_EMBED_OPERATOR=true                  # Run operator in-process

# Database
RETHINKDB_SERVICE_NAME=rethinkdb             # Service name
RETHINKDB_NAMESPACE=default                  # Namespace
RETHINKDB_ADDR=                              # Override address (optional)

# MetaGuildNet
METAGN_SETUP_MODE=auto                       # auto|interactive|minimal
METAGN_VERIFY_TIMEOUT=300                    # Verification timeout (sec)
METAGN_AUTO_APPROVE_ROUTES=true              # Auto-approve routes
METAGN_LOG_LEVEL=info                        # Log level
```

### Configuration File Precedence

```
1. CLI flags (highest priority)
2. .env.local (git-ignored)
3. .env (project default)
4. Environment variables
5. MetaGuildNet defaults
6. GuildNet defaults (lowest priority)
```

## Advanced Scenarios

### Multi-Node Cluster

```bash
# Define nodes in .env
CP_NODES=192.168.1.101,192.168.1.102,192.168.1.103
WK_NODES=192.168.1.104,192.168.1.105

# Run setup
make setup-talos

# The script will:
# - Generate configs for each node
# - Apply configs
# - Bootstrap first control plane
# - Join other nodes
# - Export kubeconfig
```

### Using Existing Headscale

```bash
# Get credentials from admin
# - Login server URL
# - Pre-auth key

# Set in .env
TS_LOGIN_SERVER=https://headscale.company.com
TS_AUTHKEY=<provided-key>

# Skip Headscale creation
make meta-setup-network-skip-headscale

# Or use custom mode
make router-install
make router-up
```

### Using Existing Kubernetes

```bash
# Get kubeconfig from admin
cp ~/Downloads/provided-kubeconfig ~/.guildnet/kubeconfig

# Ensure routes to cluster
# (Subnet router must advertise cluster CIDRs)

# Skip cluster creation
make meta-setup-application

# This installs:
# - CRDs
# - RethinkDB
# - Host App
```

### Production Deployment

```bash
# Use production-grade settings
export METAGN_SETUP_MODE=production

# Multi-node cluster
CP_NODES=192.168.1.101,192.168.1.102,192.168.1.103
WK_NODES=192.168.1.104,192.168.1.105,192.168.1.106

# Persistent storage
WORKSPACE_STORAGE_CLASS=longhorn
RETHINKDB_STORAGE_CLASS=longhorn
RETHINKDB_STORAGE_SIZE=100Gi

# LoadBalancer settings
WORKSPACE_LB=true
WORKSPACE_LB_POOL=production-pool
METALLB_ADDRESS_POOL=192.168.1.200-192.168.1.250

# TLS certificates
CERT_MANAGER_ISSUER=letsencrypt-prod
WORKSPACE_DOMAIN=workspaces.company.com

# Run setup
make meta-setup
```

## Troubleshooting

### Setup Fails at Network Layer

**Symptom**: Headscale container fails to start

```bash
# Check Docker
docker ps -a | grep headscale

# Check logs
docker logs guildnet-headscale

# Common issues:
# 1. Port 8080 already in use
#    Solution: Change HEADSCALE_PORT in .env

# 2. LAN IP not detected
#    Solution: Set TS_LOGIN_SERVER manually
```

**Symptom**: Tailscale won't connect

```bash
# Check Tailscale status
tailscale status

# Check if daemon is running
systemctl status tailscaled
# or
sudo tailscale status

# Common issues:
# 1. tailscaled not running
#    Solution: make router-daemon-sudo

# 2. Login server unreachable
#    Solution: Verify TS_LOGIN_SERVER is accessible
#    curl $TS_LOGIN_SERVER/health

# 3. Invalid auth key
#    Solution: Regenerate with make headscale-bootstrap
```

### Setup Fails at Cluster Layer

**Symptom**: Talos nodes won't bootstrap

```bash
# Check Talos logs
talosctl -n $CP_NODES logs

# Check if nodes are accessible
talosctl -n $CP_NODES version

# Common issues:
# 1. Nodes not reachable
#    Solution: Verify network connectivity

# 2. Config generation failed
#    Solution: Check talosctl version
#    talosctl version

# 3. Bootstrap timeout
#    Solution: Increase timeout in scripts/setup-talos.sh
```

**Symptom**: Kubernetes API not reachable

```bash
# Check Talos service status
talosctl -n $CP_NODES service kubelet status

# Check if API is bound
talosctl -n $CP_NODES get service kubernetes

# Common issues:
# 1. API not started
#    Solution: Wait longer, check logs

# 2. Firewall blocking
#    Solution: Allow port 6443

# 3. Kubeconfig wrong
#    Solution: Re-export
#    talosctl -n $CP_NODES kubeconfig ~/.guildnet/kubeconfig
```

### Setup Fails at Application Layer

**Symptom**: Host App won't start

```bash
# Check build
ls -lh bin/hostapp

# Try running directly
./bin/hostapp serve

# Common issues:
# 1. Port already in use
#    Solution: Change LISTEN_LOCAL

# 2. Kubeconfig not found
#    Solution: Export KUBECONFIG=~/.guildnet/kubeconfig

# 3. Certificates missing
#    Solution: make regen-certs
```

**Symptom**: Can't access UI

```bash
# Check if Host App is running
curl -k https://127.0.0.1:8080/healthz

# Check Tailscale IP
tailscale status | grep $(hostname)

# Common issues:
# 1. Certificate error
#    Solution: Accept self-signed cert or regenerate with SANs
#    ./scripts/generate-server-cert.sh -H "localhost,127.0.0.1,<ts-ip>" -f

# 2. CORS error
#    Solution: Verify FRONTEND_ORIGIN matches access URL

# 3. Tailnet listener not started
#    Solution: Check TS_AUTHKEY is valid
```

### Verification Failures

```bash
# Run step-by-step verification
make meta-verify-step-by-step

# This runs each layer independently and shows details

# Export diagnostic bundle
make meta-export-diagnostics

# Creates MetaGuildNet/diagnostics-<timestamp>.tar.gz with:
# - All logs
# - Configuration files
# - System information
# - Network traces
```

## Reset and Cleanup

### Reset Single Component

```bash
# Reset network only
make headscale-down
make router-down

# Reset cluster only
make talos-reset

# Reset application only
make stop-all
```

### Full Reset

```bash
# Stop everything
make meta-cleanup

# Remove persistent data
rm -rf ~/.guildnet
rm -rf ~/.talos

# Remove containers
docker rm -f guildnet-headscale
```

### Partial Reset

```bash
# Keep network, reset cluster and app
make talos-reset
make stop-all
make deploy-k8s-addons
make run
```

## Next Steps

After successful setup:

1. **Verify Everything Works**
   ```bash
   make meta-verify
   ```

2. **Create First Workspace**
   ```bash
   bash MetaGuildNet/examples/basic/create-workspace.sh
   ```

3. **Access UI**
   - Local: https://127.0.0.1:8080
   - Tailnet: https://<ts-hostname>:443

4. **Review Documentation**
   - [Verification Guide](VERIFICATION.md)
   - [Architecture](ARCHITECTURE.md)
   - [Examples](../examples/)

## Getting Help

- **Documentation**: Check all docs in `MetaGuildNet/docs/`
- **Examples**: Browse `MetaGuildNet/examples/`
- **Diagnostics**: Run `make meta-diagnose`
- **Issues**: Open issue in fork repository
- **Community**: Join discussions

## References

### GuildNet Repository
- [Core Architecture](../../architecture.md) - Upstream system design
- [Host App Implementation](../../cmd/hostapp/main.go) - Main application entry point
- [Operator Code](../../internal/operator/) - Kubernetes operator logic
- [Makefile](../../Makefile) - Build targets and automation commands
- [Kubernetes Manifests](../../k8s/) - Deployment configurations
- [Configuration Files](../../config/) - CRDs and settings

### MetaGuildNet Extensions
- [Architecture Guide](ARCHITECTURE.md) - Fork-specific design decisions
- [Verification Guide](VERIFICATION.md) - Testing and health check procedures
- [Contributing Guide](CONTRIBUTING.md) - Development and testing workflows
- [Upstream Sync](UPSTREAM_SYNC.md) - Synchronization with upstream GuildNet

### External Resources
- [Docker Documentation](https://docs.docker.com/) - Container runtime
- [Kubernetes Documentation](https://kubernetes.io/docs/) - Container orchestration
- [Talos Linux](https://www.talos.dev/) - Kubernetes-focused OS
- [Headscale Documentation](https://headscale.net/) - Tailscale control server
- [Tailscale Documentation](https://tailscale.com/kb/) - Zero-trust networking
- [RethinkDB Documentation](https://rethinkdb.com/docs/) - Distributed database

### Scripts and Tools
- [Setup Scripts](../scripts/setup/) - Installation automation
- [Verification Scripts](../scripts/verify/) - Health checking
- [Utility Scripts](../scripts/utils/) - Helper functions
- [Example Scripts](../examples/) - Usage demonstrations

---

**Tip**: Save your successful configuration as a template:
```bash
cp .env MetaGuildNet/templates/my-setup.env
```

