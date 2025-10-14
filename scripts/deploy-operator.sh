#!/usr/bin/env bash
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$ROOT/.env" ] && . "$ROOT/.env"
export KUBECONFIG="${KUBECONFIG:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}"

NAMESPACE=${OPERATOR_NAMESPACE:-guildnet-system}
IMG_PULL_SECRET=${K8S_IMAGE_PULL_SECRET:-}
IMAGE=${OPERATOR_IMAGE:-ghcr.io/your/module/hostapp:latest}

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 1; }; }
need kubectl

# Skip silently if Kubernetes API is not reachable or kubeconfig is invalid
if ! kubectl --request-timeout=3s get --raw=/readyz >/dev/null 2>&1; then
  echo "[operator] Kubernetes API not reachable or kubeconfig invalid; skipping"
  exit 0
fi

kubectl get ns "$NAMESPACE" >/dev/null 2>&1 || kubectl create ns "$NAMESPACE"

# Minimal RBAC for controller-runtime manager across namespaces
cat <<RBAC | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: workspace-operator
  namespace: ${NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: workspace-operator
rules:
  - apiGroups: [""]
    resources: ["configmaps","secrets","events"]
    verbs: ["get","list","watch","create","update","patch"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get","list","watch","create","update","patch","delete"]
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get","list","watch","create","update","patch","delete"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get","list","watch","create","update","patch","delete"]
  - apiGroups: ["guildnet.io"]
    resources: ["workspaces","workspaces/status"]
    verbs: ["get","list","watch","create","update","patch","delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: workspace-operator
subjects:
- kind: ServiceAccount
  name: workspace-operator
  namespace: ${NAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: workspace-operator
RBAC

# Operator Deployment with explicit operator mode (override ENTRYPOINT)
if [ -n "$IMG_PULL_SECRET" ]; then
cat <<YAML | kubectl apply -n "$NAMESPACE" -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: workspace-operator
  labels:
    app: workspace-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      app: workspace-operator
  template:
    metadata:
      labels:
        app: workspace-operator
    spec:
      serviceAccountName: workspace-operator
      imagePullSecrets:
      - name: ${IMG_PULL_SECRET}
      containers:
      - name: operator
        image: ${IMAGE}
        imagePullPolicy: IfNotPresent
        command: ["/usr/local/bin/hostapp"]
        args: ["operator"]
        env:
        - name: K8S_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        resources: {}
YAML
else
cat <<YAML | kubectl apply -n "$NAMESPACE" -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: workspace-operator
  labels:
    app: workspace-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      app: workspace-operator
  template:
    metadata:
      labels:
        app: workspace-operator
    spec:
      serviceAccountName: workspace-operator
      containers:
      - name: operator
        image: ${IMAGE}
        command: ["/usr/local/bin/hostapp"]
        args: ["operator"]
        env:
        - name: K8S_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        resources: {}
YAML
fi

echo "Operator deployment applied in namespace ${NAMESPACE}"

# If running in a local kind cluster and the image is the placeholder GHCR name, warn the user
if [ "${IMAGE}" = "ghcr.io/your/module/hostapp:latest" ]; then
  if command -v kind >/dev/null 2>&1 && [ "${USE_KIND:-0}" = "1" ]; then
    echo "[operator] NOTE: operator image is the GHCR placeholder; run 'make operator-build-load' to build+load it into kind or set OPERATOR_IMAGE to a reachable image"
  fi
fi
