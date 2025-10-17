# MetaGuildNet on macOS Setup Guide

## Overview

The default `mgn install --type local` command is designed for Linux systems using MicroK8s via `snap`. On macOS, we need to use alternative approaches.

## Prerequisites Installed

✅ kubectl (via Homebrew)
✅ Docker Desktop (already have)
✅ curl, jq (already have)

## macOS Installation Options

### Option 1: Docker Desktop with Kubernetes (Recommended for macOS)

Docker Desktop includes a single-node Kubernetes cluster that's perfect for local development.

**Steps:**

1. **Enable Kubernetes in Docker Desktop**
   ```bash
   # Open Docker Desktop
   # Go to Settings > Kubernetes
   # Check "Enable Kubernetes"
   # Click "Apply & Restart"
   ```

2. **Verify Kubernetes is running**
   ```bash
   kubectl cluster-info
   kubectl get nodes
   ```

3. **Deploy GuildNet to Docker Desktop Kubernetes**
   ```bash
   cd /Users/4d/Documents/GitHub/GuildNet
   
   # Set up the namespace
   kubectl create namespace guildnet || true
   
   # Deploy RethinkDB
   kubectl apply -f k8s/rethinkdb.yaml
   
   # Run the GuildNet host app
   ./scripts/run-hostapp.sh
   ```

### Option 2: Minikube

Minikube creates a local Kubernetes cluster in a VM or container.

**Install:**
```bash
brew install minikube

# Start minikube
minikube start

# Verify
kubectl get nodes
```

**Then deploy GuildNet:**
```bash
cd /Users/4d/Documents/GitHub/GuildNet
kubectl apply -f k8s/rethinkdb.yaml
./scripts/run-hostapp.sh
```

### Option 3: Kind (Kubernetes in Docker)

Kind runs Kubernetes clusters in Docker containers.

**Install:**
```bash
brew install kind

# Create a cluster
kind create cluster --name guildnet

# Verify
kubectl cluster-info --context kind-guildnet
```

**Then deploy GuildNet:**
```bash
cd /Users/4d/Documents/GitHub/GuildNet
kubectl apply -f k8s/rethinkdb.yaml
./scripts/run-hostapp.sh
```

### Option 4: Direct Host App (No Kubernetes)

For testing MetaGuildNet CLI without a full cluster:

```bash
cd /Users/4d/Documents/GitHub/GuildNet

# Just run the host app (it will use embedded database)
go run cmd/hostapp/main.go
```

## Recommended: Docker Desktop

For macOS, **Docker Desktop with Kubernetes** is the easiest and most stable option:

1. ✅ Already have Docker Desktop installed
2. ✅ kubectl is now installed
3. ⚠️ Just need to enable Kubernetes in Docker Desktop settings

## Quick Start (Docker Desktop)

```bash
# 1. Enable Kubernetes in Docker Desktop
#    Settings > Kubernetes > Enable Kubernetes

# 2. Verify cluster is ready
kubectl get nodes

# 3. Deploy GuildNet
cd /Users/4d/Documents/GitHub/GuildNet
kubectl create namespace guildnet
kubectl apply -f k8s/rethinkdb.yaml

# 4. Wait for RethinkDB to be ready
kubectl wait --for=condition=ready pod -l app=rethinkdb -n guildnet --timeout=300s

# 5. Run the host app
./scripts/run-hostapp.sh

# 6. Test MetaGuildNet CLI
mgn verify all
mgn viz
```

## Verification

Once you have Kubernetes running and GuildNet deployed:

```bash
# Verify system
mgn verify all

# Check cluster status
mgn cluster list

# Launch dashboard
mgn viz

# Run full validation
cd /Users/4d/Documents/GitHub/GuildNet/metaguildnet
./run.sh
```

## Troubleshooting

### Port 443 in use

If port 443 is in use (shown in prerequisites check), GuildNet will still work on port 8090.

### Kubernetes not starting

If Docker Desktop Kubernetes won't start:
```bash
# Reset Kubernetes cluster in Docker Desktop
# Settings > Kubernetes > Reset Kubernetes Cluster
```

### No cluster available

If kubectl can't find a cluster:
```bash
# Check Docker Desktop is running
# Verify Kubernetes is enabled in Docker Desktop settings
kubectl config get-contexts
```

## Summary

**For macOS users:**
- ✅ kubectl: Installed
- ⚠️ snap/MicroK8s: Not available on macOS (Linux only)
- ✅ Alternative: Use Docker Desktop, Minikube, or Kind
- 🎯 Recommended: Docker Desktop with Kubernetes enabled

**Next Steps:**
1. Enable Kubernetes in Docker Desktop
2. Deploy GuildNet to your local cluster
3. Use MetaGuildNet CLI to manage it

```bash
mgn verify all
mgn cluster list
mgn viz
```

