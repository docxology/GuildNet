package model

import "time"

type Port struct {
	Port int    `json:"port"`
	Name string `json:"name,omitempty"`
}

type Resources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type Server struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Status    string            `json:"status"`
	Node      string            `json:"node,omitempty"`
	CreatedAt string            `json:"created_at,omitempty"`
	UpdatedAt string            `json:"updated_at,omitempty"`
	Ports     []Port            `json:"ports,omitempty"`
	Resources *Resources        `json:"resources,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Events    []Event           `json:"events,omitempty"`
	URL       string            `json:"url,omitempty"`
}

type Event struct {
	T       string `json:"t,omitempty"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
	Status  string `json:"status,omitempty"`
}

type LogLine struct {
	T   string `json:"t,omitempty"`
	LVL string `json:"lvl,omitempty"`
	MSG string `json:"msg,omitempty"`
}

func NowISO() string { return time.Now().UTC().Format(time.RFC3339) }

// JobSpec mirrors UI expectations for launches.
type JobSpec struct {
	Name      string            `json:"name,omitempty"`
	Image     string            `json:"image"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Resources *Resources        `json:"resources,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Expose    []Port            `json:"expose,omitempty"`
}

type JobAccepted struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// DeployImage describes an image option the backend exposes for the UI to list.
type DeployImage struct {
	Label       string `json:"label"`
	Image       string `json:"image"`
	Description string `json:"description,omitempty"`
}

// AgentRecord represents a gateway/agent presence in the overlay network.
type AgentRecord struct {
	ID           string         `json:"id"`
	Org          string         `json:"org,omitempty"`
	Hostname     string         `json:"hostname,omitempty"`
	IP           string         `json:"ip"` // tailnet 100.x or reachable IP
	Ports        map[string]int `json:"ports,omitempty"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Version      string         `json:"version,omitempty"`
	LastSeen     string         `json:"last_seen"`
}

type ResolveResponse struct {
	IP        string         `json:"ip"`
	Ports     map[string]int `json:"ports,omitempty"`
	ExpiresAt string         `json:"expires_at,omitempty"`
}

// Orchestration data model (stored in local DB)

type Org struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type HeadscaleInstance struct {
	ID          string `json:"id"`
	OrgID       string `json:"orgId,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type"` // managed|external
	Endpoint    string `json:"endpoint,omitempty"`
	State       string `json:"state"`
	ConfigJSON  string `json:"configJSON,omitempty"`
	Credentials string `json:"credentials,omitempty"` // encrypted blob
	CreatedAt   string `json:"createdAt"`
}

type TailnetNamespace struct {
	ID          string   `json:"id"`
	HeadscaleID string   `json:"headscaleId"`
	Name        string   `json:"name"`
	ACLsJSON    string   `json:"aclsJSON,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	CreatedAt   string   `json:"createdAt"`
}

type PreAuthKey struct {
	ID          string  `json:"id"`
	HeadscaleID string  `json:"headscaleId"`
	NamespaceID string  `json:"namespaceId"`
	Tag         *string `json:"tag,omitempty"`
	Value       string  `json:"value"` // encrypted
	ExpiresAt   *string `json:"expiresAt,omitempty"`
	LastUsedAt  *string `json:"lastUsedAt,omitempty"`
}

type TalosCluster struct {
	ID                 string `json:"id"`
	OrgID              string `json:"orgId"`
	HeadscaleID        string `json:"headscaleId"`
	Name               string `json:"name"`
	PodCIDR            string `json:"podCIDR"`
	SvcCIDR            string `json:"svcCIDR"`
	Version            string `json:"version"`
	DesiredReplicasCP  int    `json:"desiredReplicasCP"`
	DesiredReplicasW   int    `json:"desiredReplicasW"`
	SubnetRouterNodeID string `json:"subnetRouterNodeId,omitempty"`
	State              string `json:"state"`
	CreatedAt          string `json:"createdAt"`
}

type TalosNode struct {
	ID            string   `json:"id"`
	ClusterID     string   `json:"clusterId"`
	Role          string   `json:"role"` // cp|worker
	ProviderRef   string   `json:"providerRef,omitempty"`
	TailscaleTags []string `json:"tailscaleTags,omitempty"`
	TailscaleIP   string   `json:"tailscaleIP,omitempty"`
	KubeNodeName  string   `json:"kubeNodeName,omitempty"`
	State         string   `json:"state"`
	CreatedAt     string   `json:"createdAt"`
}

type Credential struct {
	ID        string `json:"id"`
	ScopeType string `json:"scopeType"`
	ScopeID   string `json:"scopeId"`
	Kind      string `json:"kind"`
	Value     string `json:"value"` // encrypted
	RotatedAt string `json:"rotatedAt,omitempty"`
}

type Job struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	SpecJSON  string  `json:"specJSON"`
	Status    string  `json:"status"` // queued|running|succeeded|failed|canceled
	Progress  float64 `json:"progress"`
	LogsRef   string  `json:"logsRef,omitempty"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

type OrchestrationAuditEvent struct {
	ID         string `json:"id"`
	Actor      string `json:"actor"`
	Action     string `json:"action"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	DiffJSON   string `json:"diffJSON,omitempty"`
	TS         string `json:"ts"`
}
