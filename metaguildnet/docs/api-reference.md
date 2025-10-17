# MetaGuildNet API Reference

Complete API documentation for Go SDK and Python CLI.

## Go SDK

Import: `github.com/docxology/GuildNet/metaguildnet/sdk/go/client`

### Client

#### NewClient

```go
func NewClient(baseURL, token string, opts ...ClientOption) *Client
```

Creates a new MetaGuildNet client.

**Parameters:**
- `baseURL`: GuildNet Host App URL (e.g., `https://localhost:8090`)
- `token`: Optional API token for authentication
- `opts`: Optional configuration options

**Options:**
- `WithHTTPClient(httpClient)`: Custom HTTP client
- `WithTimeout(duration)`: Request timeout
- `WithMaxRetries(n)`: Maximum retry attempts
- `WithRetryBackoff(duration)`: Retry backoff duration

**Example:**
```go
c := client.NewClient("https://localhost:8090", "",
    client.WithTimeout(30*time.Second),
    client.WithMaxRetries(3))
```

### Cluster Operations

#### List Clusters

```go
func (c *ClusterClient) List(ctx context.Context) ([]Cluster, error)
```

Returns all registered clusters.

#### Get Cluster

```go
func (c *ClusterClient) Get(ctx context.Context, id string) (*Cluster, error)
```

Returns details for a specific cluster.

#### Bootstrap Cluster

```go
func (c *ClusterClient) Bootstrap(ctx context.Context, kubeconfig []byte) (string, error)
```

Bootstrap a new cluster with the provided kubeconfig.

**Returns:** Cluster ID

#### Update Settings

```go
func (c *ClusterClient) UpdateSettings(ctx context.Context, id string, settings ClusterSettings) error
```

Update cluster-specific settings.

**ClusterSettings:**
```go
type ClusterSettings struct {
    Name              string
    Namespace         string
    APIProxyURL       string
    PreferPodProxy    bool
    UsePortForward    bool
    IngressDomain     string
    WorkspaceLBEnabled bool
    // ... see internal/settings/settings.go for complete list
}
```

#### Get Kubeconfig

```go
func (c *ClusterClient) GetKubeconfig(ctx context.Context, id string) ([]byte, error)
```

Retrieve the stored kubeconfig for a cluster.

### Workspace Operations

#### List Workspaces

```go
func (w *WorkspaceClient) List(ctx context.Context) ([]Workspace, error)
```

List all workspaces in the cluster.

#### Create Workspace

```go
func (w *WorkspaceClient) Create(ctx context.Context, spec WorkspaceSpec) (*Workspace, error)
```

Create a new workspace.

**WorkspaceSpec:**
```go
type WorkspaceSpec struct {
    Name   string
    Image  string
    Env    []EnvVar
    Ports  []Port
    Args   []string
    Labels map[string]string
}

type EnvVar struct {
    Name  string
    Value string
}

type Port struct {
    Name          string
    ContainerPort int32
    Protocol      string  // TCP or UDP
}
```

#### Get Workspace

```go
func (w *WorkspaceClient) Get(ctx context.Context, name string) (*Workspace, error)
```

Get workspace details.

**Workspace:**
```go
type Workspace struct {
    ID            string
    Name          string
    Image         string
    Status        string  // Pending, Running, Failed
    ReadyReplicas int32
    ServiceDNS    string
    ServiceIP     string
    ExternalURL   string
    CreatedAt     time.Time
}
```

#### Delete Workspace

```go
func (w *WorkspaceClient) Delete(ctx context.Context, name string) error
```

Delete a workspace.

#### Get Logs

```go
func (w *WorkspaceClient) Logs(ctx context.Context, name string, opts LogOptions) ([]LogLine, error)
```

Retrieve workspace logs.

**LogOptions:**
```go
type LogOptions struct {
    TailLines int
    Follow    bool
    Since     time.Time
}
```

#### Stream Logs

```go
func (w *WorkspaceClient) StreamLogs(ctx context.Context, name string) (<-chan LogEvent, error)
```

Stream logs in real-time.

### Database Operations

#### List Databases

```go
func (d *DatabaseClient) List(ctx context.Context) ([]Database, error)
```

List all databases in the cluster.

#### Create Database

```go
func (d *DatabaseClient) Create(ctx context.Context, name, description string) (*Database, error)
```

Create a new database.

#### Get Tables

```go
func (d *DatabaseClient) Tables(ctx context.Context, dbID string) ([]Table, error)
```

List tables in a database.

#### Create Table

```go
func (d *DatabaseClient) CreateTable(ctx context.Context, dbID string, table Table) error
```

Create a new table with schema.

**Table:**
```go
type Table struct {
    Name       string
    PrimaryKey string
    Schema     []ColumnDef
}

type ColumnDef struct {
    Name     string
    Type     string  // string, number, boolean, timestamp, etc.
    Required bool
    Unique   bool
    Indexed  bool
}
```

#### Query Rows

```go
func (d *DatabaseClient) Query(ctx context.Context, dbID, table, orderBy string, limit int, cursor string, forward bool) ([]map[string]any, string, error)
```

Query rows from a table.

**Returns:** rows, next cursor, error

#### Insert Rows

```go
func (d *DatabaseClient) InsertRows(ctx context.Context, dbID, table string, rows []map[string]any) ([]string, error)
```

Insert multiple rows.

**Returns:** row IDs

### Health Operations

#### Global Health

```go
func (h *HealthClient) Global(ctx context.Context) (*HealthSummary, error)
```

Get overall system health.

**HealthSummary:**
```go
type HealthSummary struct {
    Healthy      bool
    Headscale    []HeadscaleStatus
    Clusters     []ClusterHealth
    LastChecked  time.Time
}
```

#### Cluster Health

```go
func (h *HealthClient) Cluster(ctx context.Context, id string) (*ClusterStatus, error)
```

Get cluster-specific health status.

**ClusterStatus:**
```go
type ClusterStatus struct {
    ClusterID         string
    KubeconfigPresent bool
    KubeconfigValid   bool
    K8sReachable      bool
    K8sError          string
    PFAvailable       bool
    TSAvailable       bool
    RecommendedAction string
    LastChecked       time.Time
}
```

#### Published Services

```go
func (h *HealthClient) Published(ctx context.Context, clusterID string) ([]PublishedService, error)
```

List published services (tsnet listeners).

### Testing Utilities

#### WaitForWorkspaceReady

```go
func WaitForWorkspaceReady(client *client.Client, clusterID, name string, timeout time.Duration) error
```

Wait for workspace to reach Running status.

#### AssertClusterHealthy

```go
func AssertClusterHealthy(t *testing.T, client *client.Client, clusterID string)
```

Assert cluster is healthy (fails test if not).

#### AssertWorkspaceRunning

```go
func AssertWorkspaceRunning(t *testing.T, client *client.Client, clusterID, name string)
```

Assert workspace is in Running state.

#### NewTestCluster

```go
func NewTestCluster(t *testing.T) *TestCluster
```

Create a test cluster that auto-cleans up.

**TestCluster:**
```go
type TestCluster struct {
    ID      string
    Cleanup func()
}
```

---

## Python CLI

Command: `mgn`

### Global Options

```
-h, --help              Show help
--api-url URL           GuildNet API URL (default: https://localhost:8090)
--token TOKEN           API authentication token
--config FILE           Config file path (default: ~/.metaguildnet/config.yaml)
--format FORMAT         Output format: json, yaml, table (default: table)
-v, --verbose           Verbose output
```

### Cluster Commands

#### mgn cluster list

List all clusters.

```bash
mgn cluster list [--format json|yaml|table]
```

#### mgn cluster get

Get cluster details.

```bash
mgn cluster get <cluster-id> [-o yaml]
```

#### mgn cluster status

Show cluster health status.

```bash
mgn cluster status <cluster-id>
```

#### mgn cluster bootstrap

Bootstrap a new cluster.

```bash
mgn cluster bootstrap --kubeconfig <path> [--name <name>]
```

#### mgn cluster update

Update cluster settings.

```bash
mgn cluster update <cluster-id> --setting <key=value>
```

### Workspace Commands

#### mgn workspace list

List workspaces in a cluster.

```bash
mgn workspace list <cluster-id>
```

#### mgn workspace create

Create a new workspace.

```bash
mgn workspace create <cluster-id> \
  --name <name> \
  --image <image> \
  [--env KEY=VALUE] \
  [--port PORT] \
  [--arg ARG]
```

#### mgn workspace get

Get workspace details.

```bash
mgn workspace get <cluster-id> <name> [-o yaml]
```

#### mgn workspace delete

Delete a workspace.

```bash
mgn workspace delete <cluster-id> <name>
```

#### mgn workspace logs

View workspace logs.

```bash
mgn workspace logs <cluster-id> <name> \
  [--tail N] \
  [--follow] \
  [--since TIMESTAMP]
```

#### mgn workspace wait

Wait for workspace to be ready.

```bash
mgn workspace wait <cluster-id> <name> [--timeout 5m]
```

#### mgn workspace test

Run health checks on workspace.

```bash
mgn workspace test <cluster-id> <name> \
  --http-check <path> \
  [--expected-status 200]
```

### Database Commands

#### mgn db list

List databases.

```bash
mgn db list <cluster-id>
```

#### mgn db create

Create database.

```bash
mgn db create <cluster-id> <name> [--description DESC]
```

#### mgn db table create

Create table.

```bash
mgn db table create <cluster-id> <db-id> <table-name> \
  --schema <col:type:flags,...> \
  --primary-key <column>
```

Schema format: `name:string:required,email:string:required:unique`

#### mgn db insert

Insert rows.

```bash
mgn db insert <cluster-id> <db-id> <table> \
  --data '{"key":"value"}' \
  [--file data.json]
```

#### mgn db query

Query rows.

```bash
mgn db query <cluster-id> <db-id> <table> \
  [--limit N] \
  [--order-by COLUMN] \
  [--format json]
```

### Installation Commands

#### mgn install

Run automated installation.

```bash
mgn install \
  [--type local|bare-metal] \
  [--cluster-name NAME] \
  [--skip-verify]
```

### Verification Commands

#### mgn verify system

Verify system prerequisites.

```bash
mgn verify system
```

Checks: OS, packages, permissions, disk space

#### mgn verify network

Verify network connectivity.

```bash
mgn verify network
```

Checks: DNS, internet, Tailscale, certificates

#### mgn verify kubernetes

Verify Kubernetes cluster.

```bash
mgn verify kubernetes
```

Checks: API server, nodes, resources, addons

#### mgn verify guildnet

Verify GuildNet installation.

```bash
mgn verify guildnet
```

Checks: Host App, operator, CRDs, databases

#### mgn verify all

Run all verification checks.

```bash
mgn verify all
```

### Visualization Commands

#### mgn viz

Launch real-time dashboard.

```bash
mgn viz [--cluster CLUSTER-ID] [--refresh 5]
```

Interactive dashboard showing:
- Cluster status
- Workspace health
- Resource usage
- Live logs

---

## REST API

MetaGuildNet wraps the GuildNet Host App API. See [GuildNet API.md](../../API.md) for complete endpoint documentation.

### Common Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/deploy/clusters` | GET | List clusters |
| `/api/cluster/{id}/servers` | GET | List workspaces |
| `/api/cluster/{id}/workspaces` | POST | Create workspace |
| `/api/cluster/{id}/workspaces/{name}` | DELETE | Delete workspace |
| `/api/cluster/{id}/db` | GET | List databases |
| `/api/health` | GET | System health |
| `/api/cluster/{id}/health` | GET | Cluster health |

### Authentication

Include token in header:

```
Authorization: Bearer <token>
```

Or use loopback exemption (127.0.0.1 when no token configured).

---

## Error Handling

### Go SDK Errors

```go
import "github.com/docxology/GuildNet/metaguildnet/sdk/go/client"

var (
    ErrNotFound      = errors.New("resource not found")
    ErrUnauthorized  = errors.New("unauthorized")
    ErrTimeout       = errors.New("request timeout")
    ErrServerError   = errors.New("server error")
)
```

Check errors:

```go
if errors.Is(err, client.ErrNotFound) {
    // Handle not found
}
```

### Python CLI Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid usage |
| 3 | Network error |
| 4 | API error |
| 5 | Validation error |

---

## Configuration

### Go SDK

```go
c := client.NewClient(
    os.Getenv("MGN_API_URL"),
    os.Getenv("MGN_API_TOKEN"),
    client.WithTimeout(30*time.Second),
)
```

### Python CLI

Configuration file: `~/.metaguildnet/config.yaml`

```yaml
api:
  base_url: https://localhost:8090
  token: ""
  timeout: 30

defaults:
  cluster: production
  format: table

logging:
  level: info
  file: ~/.metaguildnet/logs/mgn.log
```

Environment variables override config file:

```bash
export MGN_API_URL=https://guildnet.example.com:8090
export MGN_API_TOKEN=secret
export MGN_DEFAULT_CLUSTER=production
```

---

## See Also

- [Getting Started](getting-started.md) - Installation and first steps
- [Concepts](concepts.md) - Architecture and patterns
- [Examples](examples.md) - Detailed walkthroughs
- [GuildNet API.md](../../API.md) - Full GuildNet API documentation

