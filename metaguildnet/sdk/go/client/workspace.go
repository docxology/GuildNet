package client

import (
	"context"
	"fmt"
	"time"
)

// WorkspaceClient handles workspace operations for a specific cluster
type WorkspaceClient struct {
	client    *Client
	clusterID string
}

// Workspace represents a GuildNet workspace
type Workspace struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	Status        string            `json:"status"` // Pending, Running, Failed, Terminating
	ReadyReplicas int32             `json:"readyReplicas"`
	ServiceDNS    string            `json:"serviceDNS,omitempty"`
	ServiceIP     string            `json:"serviceIP,omitempty"`
	ExternalURL   string            `json:"externalURL,omitempty"`
	ProxyTarget   string            `json:"proxyTarget,omitempty"`
	Ports         []WorkspacePort   `json:"ports,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	CreatedAt     time.Time         `json:"createdAt,omitempty"`
}

// WorkspaceSpec defines workspace creation parameters
type WorkspaceSpec struct {
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Env    []EnvVar          `json:"env,omitempty"`
	Ports  []WorkspacePort   `json:"ports,omitempty"`
	Args   []string          `json:"args,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Notes  string            `json:"notes,omitempty"`
}

// EnvVar represents an environment variable
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WorkspacePort represents a container port
type WorkspacePort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"` // TCP or UDP
}

// LogLine represents a single log line
type LogLine struct {
	Timestamp time.Time `json:"timestamp"`
	Line      string    `json:"line"`
	Source    string    `json:"source,omitempty"` // pod name
}

// LogOptions configures log retrieval
type LogOptions struct {
	TailLines int
	Follow    bool
	Since     time.Time
}

// LogEvent represents a streaming log event
type LogEvent struct {
	Timestamp time.Time `json:"ts"`
	Message   string    `json:"msg"`
	Level     string    `json:"level,omitempty"`
	Pod       string    `json:"pod,omitempty"`
}

// List returns all workspaces in the cluster
func (wc *WorkspaceClient) List(ctx context.Context) ([]Workspace, error) {
	var response struct {
		Servers []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Image  string `json:"image"`
			Status string `json:"status"`
			Ports  []struct {
				Name string `json:"name,omitempty"`
				Port int    `json:"port"`
			} `json:"ports"`
		} `json:"servers"`
	}

	err := wc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/servers", wc.clusterID), &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	workspaces := make([]Workspace, len(response.Servers))
	for i, s := range response.Servers {
		ports := make([]WorkspacePort, len(s.Ports))
		for j, p := range s.Ports {
			ports[j] = WorkspacePort{
				Name:          p.Name,
				ContainerPort: int32(p.Port),
			}
		}

		workspaces[i] = Workspace{
			ID:     s.ID,
			Name:   s.Name,
			Image:  s.Image,
			Status: s.Status,
			Ports:  ports,
		}
	}

	return workspaces, nil
}

// Create creates a new workspace
func (wc *WorkspaceClient) Create(ctx context.Context, spec WorkspaceSpec) (*Workspace, error) {
	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}

	err := wc.client.post(ctx, fmt.Sprintf("/api/cluster/%s/workspaces", wc.clusterID), spec, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Return minimal workspace info
	return &Workspace{
		ID:     response.ID,
		Name:   spec.Name,
		Image:  spec.Image,
		Status: response.Status,
	}, nil
}

// Get retrieves workspace details
func (wc *WorkspaceClient) Get(ctx context.Context, name string) (*Workspace, error) {
	var response map[string]interface{}

	err := wc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/workspaces/%s", wc.clusterID, name), &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Parse response into Workspace struct
	ws := &Workspace{
		Name: name,
	}

	if spec, ok := response["spec"].(map[string]interface{}); ok {
		if image, ok := spec["image"].(string); ok {
			ws.Image = image
		}
	}

	if status, ok := response["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			ws.Status = phase
		}
		if serviceDNS, ok := status["serviceDNS"].(string); ok {
			ws.ServiceDNS = serviceDNS
		}
		if serviceIP, ok := status["serviceIP"].(string); ok {
			ws.ServiceIP = serviceIP
		}
		if externalURL, ok := status["externalURL"].(string); ok {
			ws.ExternalURL = externalURL
		}
	}

	return ws, nil
}

// Delete deletes a workspace
func (wc *WorkspaceClient) Delete(ctx context.Context, name string) error {
	err := wc.client.delete(ctx, fmt.Sprintf("/api/cluster/%s/workspaces/%s", wc.clusterID, name))
	if err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}

	return nil
}

// Logs retrieves workspace logs
func (wc *WorkspaceClient) Logs(ctx context.Context, name string, opts LogOptions) ([]LogLine, error) {
	path := fmt.Sprintf("/api/cluster/%s/workspaces/%s/logs", wc.clusterID, name)

	// Add query parameters
	if opts.TailLines > 0 {
		path += fmt.Sprintf("?tail=%d", opts.TailLines)
	}

	var response []struct {
		Timestamp string `json:"timestamp"`
		Line      string `json:"line"`
		Pod       string `json:"pod,omitempty"`
	}

	err := wc.client.get(ctx, path, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	logs := make([]LogLine, len(response))
	for i, l := range response {
		ts, _ := time.Parse(time.RFC3339, l.Timestamp)
		logs[i] = LogLine{
			Timestamp: ts,
			Line:      l.Line,
			Source:    l.Pod,
		}
	}

	return logs, nil
}

// StreamLogs streams workspace logs in real-time
// Note: This is a simplified implementation. For production, use websockets or SSE.
func (wc *WorkspaceClient) StreamLogs(ctx context.Context, name string) (<-chan LogEvent, error) {
	// This would require websocket implementation
	// For now, return a channel that polls logs
	ch := make(chan LogEvent, 100)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var lastTimestamp time.Time

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logs, err := wc.Logs(ctx, name, LogOptions{TailLines: 100})
				if err != nil {
					continue
				}

				for _, log := range logs {
					if log.Timestamp.After(lastTimestamp) {
						select {
						case ch <- LogEvent{
							Timestamp: log.Timestamp,
							Message:   log.Line,
							Pod:       log.Source,
						}:
						case <-ctx.Done():
							return
						}
						lastTimestamp = log.Timestamp
					}
				}
			}
		}
	}()

	return ch, nil
}

// Wait waits for workspace to reach Running status
func (wc *WorkspaceClient) Wait(ctx context.Context, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for workspace: %w", ctx.Err())
		case <-ticker.C:
			ws, err := wc.Get(ctx, name)
			if err != nil {
				continue
			}

			switch ws.Status {
			case "Running":
				return nil
			case "Failed":
				return fmt.Errorf("workspace failed")
			}
		}
	}
}

// ProxyURL returns the proxy URL for accessing the workspace
func (wc *WorkspaceClient) ProxyURL(name string) string {
	return fmt.Sprintf("%s/api/cluster/%s/proxy/server/%s/",
		wc.client.baseURL, wc.clusterID, name)
}
