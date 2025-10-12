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
