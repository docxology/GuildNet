#!/usr/bin/env bash
set -euo pipefail

# verify-cluster.sh
# Create a disposable kind cluster test-<rand>, generate guildnet.config, POST to hostapp /bootstrap,
# create a workspace (code-server), tail logs via SSE, exercise DB API (list/create/table/delete),
# then destroy the cluster. Verbose logging and diagnostics are written to /tmp/verify-cluster-<ts>.log

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
LOGDIR=${LOGDIR:-/tmp}
TS=$(date -u +%Y%m%dT%H%M%SZ)
LOGFILE="$LOGDIR/verify-cluster-$TS.log"
set -o pipefail

echolog() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*" | tee -a "$LOGFILE"; }

cleanup() {
  rc=$?
  # Avoid unbound variable errors if cleanup runs before CLUSTER_NAME is set
  CLUSTER_NAME=${CLUSTER_NAME:-}
  HOSTAPP_STARTED=${HOSTAPP_STARTED:-0}
  HOSTAPP_PID_FILE=${HOSTAPP_PID_FILE:-/dev/null}
  echolog "Cleaning up: attempting to destroy kind cluster '$CLUSTER_NAME'"
  # If we started a hostapp, kill it
  if [ "${HOSTAPP_STARTED}" -eq 1 ] && [ -f "${HOSTAPP_PID_FILE}" ]; then
    pid=$(cat "${HOSTAPP_PID_FILE}" 2>/dev/null || true)
    if [ -n "$pid" ]; then
      echolog "Killing hostapp pid=$pid"
      kill "$pid" 2>/dev/null || true
    fi
  fi
  if [ "$NO_DELETE" = "1" ]; then
    echolog "NO_DELETE=1; skipping cluster deletion"
  else
    if [ -n "$CLUSTER_NAME" ]; then
      if command -v kind >/dev/null 2>&1; then
        if kind get clusters | grep -qx "$CLUSTER_NAME"; then
          echolog "Deleting kind cluster $CLUSTER_NAME"
          kind delete cluster --name "$CLUSTER_NAME" 2>&1 | tee -a "$LOGFILE" || echolog "kind delete cluster failed"
        else
          echolog "No kind cluster $CLUSTER_NAME found"
        fi
      else
        echolog "Skipping kind delete: 'kind' not found in PATH"
      fi
    else
      echolog "No CLUSTER_NAME set; skipping kind delete"
    fi
  fi
  echolog "Logs saved to $LOGFILE"
  exit $rc
}

trap cleanup EXIT

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing required binary: $1" | tee -a "$LOGFILE"; exit 2; } }
need bash
need curl
need jq
need kubectl
need kind

# NO_DELETE: when set to 1, do not delete clusters at the end (useful for debugging)
NO_DELETE=${NO_DELETE:-0}

# Control whether we create a local kind cluster. Default: do NOT use kind (use real k8s).
USE_KIND=${USE_KIND:-0}

# If using kind, generate a short random suffix and cluster name; otherwise leave empty
if [ "$USE_KIND" = "1" ]; then
  RAND=$(head -c6 /dev/urandom | od -An -tx1 | tr -d ' \n')
  CLUSTER_NAME="test-$RAND"
  export KIND_CLUSTER_NAME="$CLUSTER_NAME"
else
  CLUSTER_NAME=""
fi

# Ensure RAND is always set (used for workspace/db names) even when not creating kind
RAND=${RAND:-}
if [ -z "$RAND" ]; then
  RAND=$(head -c6 /dev/urandom | od -An -tx1 | tr -d ' \n')
fi

# Track hostapp we may start during the run
HOSTAPP_STARTED=0
HOSTAPP_PID_FILE="/tmp/verify-hostapp-${RAND:-noid}.pid"



# Control whether we create a local kind cluster. Default: do NOT use kind (use real k8s).
# Set USE_KIND=1 in CI to create a disposable kind cluster.
USE_KIND=${USE_KIND:-0}
if [ "$USE_KIND" = "1" ]; then
  # Pick an available host port for the kind API server (default 6443); avoid collisions
  pick_port() {
    for p in $(seq 6443 6500); do
      if ! ss -ltn "sport = :$p" | grep -q LISTEN; then
        echo $p; return
      fi
    done
    echo 6443
  }
  KIND_API_SERVER_PORT=$(pick_port)
  export KIND_API_SERVER_PORT
  echolog "Selected KIND API server host port: $KIND_API_SERVER_PORT"
else
  echolog "USE_KIND not set; skipping local kind cluster creation and using existing kubeconfig"
fi

echolog "Starting verify-cluster run for cluster: $CLUSTER_NAME"
echolog "Logfile: $LOGFILE"

if [ "$USE_KIND" = "1" ]; then
  echolog "Step: create kind cluster"
  if ! bash "$ROOT/scripts/kind-setup.sh" 2>&1 | tee -a "$LOGFILE"; then
    echolog "kind-setup failed"; exit 3
  fi
  KUBECONFIG_OUT=${KUBECONFIG_OUT:-${GN_KUBECONFIG:-$HOME/.guildnet/kubeconfig}}
  if [ ! -f "$KUBECONFIG_OUT" ]; then
    echolog "Expected kubeconfig at $KUBECONFIG_OUT not found after kind setup"; exit 4
  fi
else
  # Use existing kubeconfig from environment if present, otherwise default location
  KUBECONFIG_OUT=${KUBECONFIG_OUT:-${GN_KUBECONFIG:-${KUBECONFIG:-$HOME/.kube/config}}}
  if [ ! -f "$KUBECONFIG_OUT" ]; then
    echolog "No kubeconfig found at $KUBECONFIG_OUT; set KUBECONFIG or USE_KIND=1 to create a kind cluster"; exit 4
  fi
fi

# Export KUBECONFIG for subsequent kubectl calls and perform a quick preflight check
export KUBECONFIG="$KUBECONFIG_OUT"
echolog "Using kubeconfig: $KUBECONFIG_OUT"
echolog "Performing Kubernetes API preflight check (timeout 5s)"
if ! kubectl --request-timeout=5s get --raw='/readyz' >/dev/null 2>&1; then
  echolog "Kubernetes API not reachable using kubeconfig $KUBECONFIG_OUT."
  echolog "Set KUBECONFIG to a reachable cluster or run with USE_KIND=1 to create a local kind cluster.";
  exit 4
fi

echolog "Step: generate guildnet.config (join file)"
JOIN_OUT="$ROOT/guildnet.config"
if ! bash "$ROOT/scripts/create_join_info.sh" --kubeconfig "$KUBECONFIG_OUT" --name "$CLUSTER_NAME" --out "$JOIN_OUT" 2>&1 | tee -a "$LOGFILE"; then
  echolog "create_join_info failed"; exit 5
fi
echolog "Join file created: $JOIN_OUT"

echolog "Step: preflight checks for target Kubernetes cluster"
if ! bash "$ROOT/scripts/verify-cluster-preflight.sh" 2>&1 | tee -a "$LOGFILE"; then
  echolog "Preflight checks failed; aborting"; exit 5
fi

SKIP_METALLB=${SKIP_METALLB:-0}
if [ "$SKIP_METALLB" = "1" ]; then
  echolog "SKIP_METALLB=1; skipping MetalLB installation"
else
  echolog "Step: deploy MetalLB into the new cluster"
  if ! bash "$ROOT/scripts/deploy-metallb.sh" 2>&1 | tee -a "$LOGFILE"; then
    echolog "deploy-metallb.sh failed (continuing to try)";
  fi
fi

echolog "Step: deploy RethinkDB StatefulSet (apply with --validate=false for compatibility)"
# Some Kubernetes API servers reject server-side validation (openapi) for CRDs or when API aggregation is limited.
# Use --validate=false to avoid failing apply on such clusters.
kubectl apply --validate=false -f "$ROOT/k8s/rethinkdb.yaml" 2>&1 | tee -a "$LOGFILE" || true

# Wait for rethinkdb pod to be Running and for logs to contain 'Server ready'
echolog "Waiting up to 180s for rethinkdb pods to be ready and server to announce readiness"
RDB_POD=""
for i in $(seq 1 60); do
  sleep 3
  RDB_POD=$(kubectl get pods -l app=rethinkdb -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
  if [ -n "$RDB_POD" ]; then
    phase=$(kubectl get pod "$RDB_POD" -o jsonpath='{.status.phase}' 2>/dev/null || true)
    ready_cnt=$(kubectl get pod "$RDB_POD" -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null || true)
    echolog "rethinkdb pod: $RDB_POD phase=$phase ready=$ready_cnt"
    if [ "${phase}" = "Running" ] && [ "$ready_cnt" = "true" ]; then
      # check logs for server ready
      if kubectl logs "$RDB_POD" --tail=200 2>/dev/null | grep -qi "Server ready"; then
        echolog "rethinkdb server ready in pod $RDB_POD"
        break
      fi
    fi
  fi
done
if [ -z "$RDB_POD" ]; then
  echolog "rethinkdb pod not found or not ready after timeout"; # continue, bootstrap may still succeed with clusterIP
fi

# Ensure at least a service address exists (ClusterIP or LoadBalancer)
RDB_SVC_CLUSTERIP=$(kubectl get svc rethinkdb -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)
RDB_SVC_LB=$(kubectl get svc rethinkdb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
echolog "RethinkDB service clusterIP=$RDB_SVC_CLUSTERIP loadbalancer=$RDB_SVC_LB"

# If no Service was created by the manifest, create a fallback ClusterIP Service so the
# Host App's ConnectForK8s can discover an in-cluster address.
if [ -z "$RDB_SVC_CLUSTERIP" ] || [ "$RDB_SVC_CLUSTERIP" = "" ]; then
  echolog "No rethinkdb Service found; creating fallback NodePort service"
  # Use NodePort so the Host App (running on the host) can reach the DB via nodeIP:nodePort
  cat <<'YAML' | kubectl apply --validate=false -f - 2>&1 | tee -a "$LOGFILE" || true
apiVersion: v1
kind: Service
metadata:
  name: rethinkdb
  labels:
    app: rethinkdb
spec:
  type: NodePort
  ports:
    - name: client
      port: 28015
      targetPort: 28015
      protocol: TCP
  selector:
    app: rethinkdb
YAML
  RDB_SVC_CLUSTERIP=$(kubectl get svc rethinkdb -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)
  echolog "After fallback creation, rethinkdb service clusterIP=$RDB_SVC_CLUSTERIP"
fi


# Hostapp endpoint (assume running locally)
HOSTAPP_URL=${HOSTAPP_URL:-https://127.0.0.1:8090}
echolog "Assuming Host App at: $HOSTAPP_URL"

# Ensure Host App process will know how to find RethinkDB service in the cluster
export RETHINKDB_SERVICE_NAME=${RETHINKDB_SERVICE_NAME:-rethinkdb}
export RETHINKDB_NAMESPACE=${RETHINKDB_NAMESPACE:-default}
echolog "Exported RETHINKDB_SERVICE_NAME=$RETHINKDB_SERVICE_NAME RETHINKDB_NAMESPACE=$RETHINKDB_NAMESPACE"

ensure_hostapp() {
  echolog "Checking hostapp /healthz"
  HC=$(curl --insecure -sS -o /dev/null -w "%{http_code}" "$HOSTAPP_URL/healthz" || true)
  if [ "$HC" = "200" ]; then
    echolog "Hostapp is already up"
    return 0
  fi
  echolog "Hostapp not reachable (healthz=$HC); starting local hostapp via scripts/run-hostapp.sh"
  nohup bash "$ROOT/scripts/run-hostapp.sh" >>"$LOGFILE" 2>&1 &
  pid=$!
  echo "$pid" > "$HOSTAPP_PID_FILE"
  HOSTAPP_STARTED=1
  echolog "Started hostapp (pid=$pid); waiting for /healthz up to 30s"
  for i in {1..30}; do
    sleep 1
    HC=$(curl --insecure -sS -o /dev/null -w "%{http_code}" "$HOSTAPP_URL/healthz" || true)
    if [ "$HC" = "200" ]; then
      echolog "Hostapp is healthy"
      return 0
    fi
  done
  echolog "Hostapp failed to become healthy in time (last status=$HC)"
  return 1
}

ensure_hostapp


echolog "Step: POST /bootstrap (with retries, forcing HTTP/1.1 to avoid h2 edge cases)"
BOOT_RESP=$(mktemp)
BOOT_OK=0
for attempt in 1 2 3 4 5; do
  echolog "Bootstrap attempt #$attempt"
  # Force HTTP/1.1 to avoid http2 stream close edge cases seen in CI
  HTTP_CODE=$(curl --http1.1 --insecure -sS -w "%{http_code}" -o "$BOOT_RESP" -X POST "$HOSTAPP_URL/api/bootstrap" \
    -H "Content-Type: application/json" --data-binary @"$JOIN_OUT" 2>&1 | tee -a "$LOGFILE" || true)
  echolog "Raw curl exit status captured, http_code="$HTTP_CODE""
  if [ "$HTTP_CODE" = "200" ]; then
    BOOT_OK=1
    break
  fi
  echolog "Bootstrap attempt #$attempt returned http_code=$HTTP_CODE; response body:"
  sed -n '1,200p' "$BOOT_RESP" | tee -a "$LOGFILE"
  sleep $((attempt * 1))
done
if [ "$BOOT_OK" -ne 1 ]; then
  echolog "Bootstrap failed after retries (last response body above). Aborting."; exit 6
fi

# Parse cluster id from bootstrap response if present (check multiple shapes)
CLUSTER_ID=$(jq -r '.clusterId // .cluster.id // .id // empty' "$BOOT_RESP" | tr -d '\n')
if [ -z "$CLUSTER_ID" ]; then
  # Fallback: attempt to list clusters from Host App
  echolog "No cluster id in bootstrap response; fetching cluster list"
  # The API exposes cluster listing under /api/deploy/clusters
  CLUSTER_LIST=$(curl --insecure -sS "$HOSTAPP_URL/api/deploy/clusters" || echo "")
  echolog "Clusters: $CLUSTER_LIST"
  CLUSTER_ID=$(echo "$CLUSTER_LIST" | jq -r '.[0].id // .[0].clusterId // empty' 2>/dev/null || true)
fi
if [ -z "$CLUSTER_ID" ]; then
  echolog "Unable to determine cluster ID from hostapp. Aborting."; exit 7
fi
echolog "Using cluster ID: $CLUSTER_ID"

echolog "Step: list servers (expect empty)"
SERVERS=$(curl --insecure -sS "$HOSTAPP_URL/api/cluster/$CLUSTER_ID/servers" | tee -a "$LOGFILE") || true
echolog "Servers: $SERVERS"

echolog "Step: create a code-server workspace via per-cluster API"
# Compose workspace spec for API
WORKSPACE_NAME="verify-cs-$RAND"
IMAGE=${VERIFY_CODESERVER_IMAGE:-"codercom/code-server:4.9.0"}
WS_PAYLOAD=$(jq -n --arg name "$WORKSPACE_NAME" --arg img "$IMAGE" '{name:$name, image:$img, ports:[{containerPort:8080,name:"http"}], env:[{name:"PASSWORD", value:"testpass"}] }')
echolog "Workspace payload: $WS_PAYLOAD"

CREATE_WS_RESP=$(mktemp)
HTTP_CODE=$(curl --insecure -sS -w "%{http_code}" -o "$CREATE_WS_RESP" -X POST "$HOSTAPP_URL/api/cluster/$CLUSTER_ID/workspaces" -H "Content-Type: application/json" --data "$WS_PAYLOAD" || true)
echolog "Create workspace HTTP code: $HTTP_CODE"
sed -n '1,200p' "$CREATE_WS_RESP" | tee -a "$LOGFILE"
if [ "$HTTP_CODE" != "200" ] && [ "$HTTP_CODE" != "201" ] && [ "$HTTP_CODE" != "202" ]; then
  echolog "Workspace creation failed (http_code=$HTTP_CODE)"; exit 8
fi

echolog "Step: wait for workspace to be reconciled and fetch proxy target"
PROXY_TARGET=""
WS_STATUS=""
for i in $(seq 1 60); do
  sleep 2
  S=$(curl --insecure -sS "$HOSTAPP_URL/api/cluster/$CLUSTER_ID/workspaces/$WORKSPACE_NAME" || echo "")
  echo "$S" | tee -a "$LOGFILE"
  # Extract workspace phase/status and proxy/service info
  WS_STATUS=$(echo "$S" | jq -r 'try .status.phase // try .status.Phase // try .status // empty' 2>/dev/null || true)
  PROXY_TARGET=$(echo "$S" | jq -r 'try .status.proxyTarget // try .status.proxy_target // try .status.externalURL // try .status.serviceDNS // try .status.serviceIP // try .proxyTarget // try .proxy_target // empty' 2>/dev/null || true)
  echolog "Workspace status: ${WS_STATUS:-<empty>} proxy_target: ${PROXY_TARGET:-<empty>}"
  if [ "$WS_STATUS" = "Running" ] || [ "$WS_STATUS" = "running" ]; then
    echolog "Workspace running"; break
  fi
  if [ -n "$PROXY_TARGET" ] && [ "$PROXY_TARGET" != "null" ]; then
    echolog "Proxy target: $PROXY_TARGET"; break
  fi
done
if [ -z "$PROXY_TARGET" ]; then
  echolog "Proxy target not available after wait"; # continue to try SSE via proxy path
  # Dump diagnostics to aid debugging
  dump_diagnostics() {
    DIAG_DIR="$LOGDIR/verify-cluster-diagnostics-$TS"
    mkdir -p "$DIAG_DIR"
    echolog "Writing diagnostics to $DIAG_DIR"
    kubectl get workspace "$WORKSPACE_NAME" -n default -o yaml >"$DIAG_DIR/workspace.yaml" 2>/dev/null || true
    kubectl get all -l "guildnet.io/workspace=$WORKSPACE_NAME" -n default -o wide >"$DIAG_DIR/kubectl-get-all.txt" 2>/dev/null || true
    kubectl describe workspace "$WORKSPACE_NAME" -n default >"$DIAG_DIR/workspace-describe.txt" 2>/dev/null || true
    kubectl -n guildnet-system get pods -l app=workspace-operator -o name >"$DIAG_DIR/operator-pods.txt" 2>/dev/null || true
    for p in $(kubectl -n guildnet-system get pods -l app=workspace-operator -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo ""); do
      kubectl -n guildnet-system logs "$p" --tail=500 >"$DIAG_DIR/operator-${p}.log" 2>/dev/null || true
    done
    kubectl get events --sort-by='.lastTimestamp' -A >"$DIAG_DIR/events.txt" 2>/dev/null || true
    echolog "Diagnostics written to $DIAG_DIR"
  }
  dump_diagnostics
fi

# If no proxy target is provided by Host App, attempt to locate a Service or Pod and forward a local port
PORT_FORWARD_PID=""
if [ -z "$PROXY_TARGET" ] || [ "$PROXY_TARGET" = "null" ]; then
  echolog "Attempting cluster-side discovery for workspace service/pod as fallback"
  # Try Service in default namespace with name == WORKSPACE_NAME
  SVC_JSON=$(kubectl get svc -o json --namespace default "$WORKSPACE_NAME" 2>/dev/null || echo "")
  # If service not found by name, try by label selector set by operator
  if [ -z "$SVC_JSON" ]; then
    SVC_NAME_BY_LABEL=$(kubectl get svc -n default -l "guildnet.io/workspace=$WORKSPACE_NAME" --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | head -n1 || true)
    if [ -n "$SVC_NAME_BY_LABEL" ]; then
      SVC_JSON=$(kubectl get svc -o json --namespace default "$SVC_NAME_BY_LABEL" 2>/dev/null || echo "")
    fi
  fi
  if [ -n "$SVC_JSON" ]; then
    SVC_TYPE=$(echo "$SVC_JSON" | jq -r '.spec.type // empty' 2>/dev/null || echo "")
    if [ "$SVC_TYPE" = "NodePort" ] || [ "$SVC_TYPE" = "LoadBalancer" ]; then
      # Prefer external access via nodeIP:nodePort
      NODE_PORT=$(echo "$SVC_JSON" | jq -r '.spec.ports[0].nodePort // empty' 2>/dev/null || echo "")
      if [ -n "$NODE_PORT" ]; then
        NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || true)
        if [ -n "$NODE_IP" ]; then
          PROXY_TARGET="$NODE_IP:$NODE_PORT"
          echolog "Found NodePort/LoadBalancer service; using proxy target $PROXY_TARGET"
        fi
      fi
    else
      # ClusterIP: we can port-forward the service locally
      PORT=$(echo "$SVC_JSON" | jq -r '.spec.ports[0].port // empty' 2>/dev/null || echo "")
      if [ -n "$PORT" ]; then
        LOCAL_PORT=18080
        echolog "Starting kubectl port-forward to service $WORKSPACE_NAME local:$LOCAL_PORT -> $PORT"
        kubectl port-forward svc/"$WORKSPACE_NAME" $LOCAL_PORT:$PORT -n default >/dev/null 2>&1 &
        PORT_FORWARD_PID=$!
        echolog "Port-forward pid=$PORT_FORWARD_PID"
        PROXY_TARGET="127.0.0.1:$LOCAL_PORT"
        # Ensure port-forward cleanup
        trap 'if [ -n "${PORT_FORWARD_PID}" ]; then kill ${PORT_FORWARD_PID} 2>/dev/null || true; fi' EXIT
      fi
    fi
  fi

  # If still not found, try to find a pod using the operator label selector and port-forward
  if [ -z "$PROXY_TARGET" ]; then
    POD_NAME=$(kubectl get pods -l "guildnet.io/workspace=$WORKSPACE_NAME" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    if [ -z "$POD_NAME" ]; then
      # try matching by pod name substring
      POD_NAME=$(kubectl get pods --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep "$WORKSPACE_NAME" | head -n1 || true)
    fi
    if [ -n "$POD_NAME" ]; then
      LOCAL_PORT=18080
      echolog "Starting kubectl port-forward to pod $POD_NAME local:$LOCAL_PORT -> 8080"
      kubectl port-forward "$POD_NAME" $LOCAL_PORT:8080 -n default >/dev/null 2>&1 &
      PORT_FORWARD_PID=$!
      echolog "Port-forward pid=$PORT_FORWARD_PID"
      PROXY_TARGET="127.0.0.1:$LOCAL_PORT"
      # Ensure port-forward cleanup
      trap 'if [ -n "${PORT_FORWARD_PID}" ]; then kill ${PORT_FORWARD_PID} 2>/dev/null || true; fi' EXIT
    else
      echolog "No pod or service found for workspace; cannot establish proxy target"
    fi
  fi
fi

echolog "Step: open SSE logs for the server (via Host App proxy)"
SSE_URL="$HOSTAPP_URL/sse/cluster/$CLUSTER_ID/servers/$WORKSPACE_NAME/logs"
echolog "SSE URL: $SSE_URL"
# Use curl to read a short window of SSE (10s)
echolog "Tailing SSE for 12 seconds to capture startup logs..."
timeout 12 curl --insecure -sS -H "Accept: text/event-stream" "$SSE_URL" | tee -a "$LOGFILE" || echolog "SSE read ended"

echolog "Step: DB API operations"
# List DBs
DBS=$(curl --insecure -sS "$HOSTAPP_URL/api/cluster/$CLUSTER_ID/db" | tee -a "$LOGFILE" || echo "")
echolog "DB list: $DBS"

# Create DB
DB_NAME="verifydb_$RAND"
CREATE_DB_PAYLOAD=$(jq -n --arg id "$DB_NAME" --arg name "$DB_NAME" '{id:$id, name:$name}')
echolog "Creating DB: $DB_NAME"
curl --insecure -sS -X POST -H "Content-Type: application/json" --data "$CREATE_DB_PAYLOAD" "$HOSTAPP_URL/api/cluster/$CLUSTER_ID/db" | tee -a "$LOGFILE" || true

sleep 1

# Create table in DB
TABLE_NAME="t1"
CREATE_TABLE_PAYLOAD=$(jq -n --arg table "$TABLE_NAME" '{name:$table, primary_key:"id"}')
echolog "Creating table $TABLE_NAME in $DB_NAME"
curl --insecure -sS -X POST -H "Content-Type: application/json" --data "$CREATE_TABLE_PAYLOAD" "$HOSTAPP_URL/api/cluster/$CLUSTER_ID/db/$DB_NAME/tables" | tee -a "$LOGFILE" || true

sleep 1

# Delete DB
echolog "Deleting DB $DB_NAME"
curl --insecure -sS -X DELETE "$HOSTAPP_URL/api/cluster/$CLUSTER_ID/db/$DB_NAME" | tee -a "$LOGFILE" || true

echolog "All tests executed"
echolog "verify-cluster.sh completed successfully"

exit 0
