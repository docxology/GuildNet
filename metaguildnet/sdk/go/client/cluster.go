package client

import (
	"context"
	"encoding/base64"
	"fmt"
)

// ClusterClient handles cluster operations
type ClusterClient struct {
	client *Client
}

// Cluster represents a GuildNet cluster
type Cluster struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Namespace          string                 `json:"namespace,omitempty"`
	APIProxyURL        string                 `json:"api_proxy_url,omitempty"`
	APIProxyForceHTTP  bool                   `json:"api_proxy_force_http,omitempty"`
	DisableAPIProxy    bool                   `json:"disable_api_proxy,omitempty"`
	PreferPodProxy     bool                   `json:"prefer_pod_proxy,omitempty"`
	UsePortForward     bool                   `json:"use_port_forward,omitempty"`
	IngressDomain      string                 `json:"ingress_domain,omitempty"`
	IngressClassName   string                 `json:"ingress_class_name,omitempty"`
	WorkspaceTLSSecret string                 `json:"workspace_tls_secret,omitempty"`
	CertManagerIssuer  string                 `json:"cert_manager_issuer,omitempty"`
	ImagePullSecret    string                 `json:"image_pull_secret,omitempty"`
	WorkspaceLBEnabled bool                   `json:"workspace_lb_enabled,omitempty"`
	OrgID              string                 `json:"org_id,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// ClusterSettings represents configurable cluster settings
type ClusterSettings struct {
	Name               string `json:"name,omitempty"`
	Namespace          string `json:"namespace,omitempty"`
	APIProxyURL        string `json:"api_proxy_url,omitempty"`
	APIProxyForceHTTP  bool   `json:"api_proxy_force_http,omitempty"`
	DisableAPIProxy    bool   `json:"disable_api_proxy,omitempty"`
	PreferPodProxy     bool   `json:"prefer_pod_proxy,omitempty"`
	UsePortForward     bool   `json:"use_port_forward,omitempty"`
	IngressDomain      string `json:"ingress_domain,omitempty"`
	IngressClassName   string `json:"ingress_class_name,omitempty"`
	WorkspaceTLSSecret string `json:"workspace_tls_secret,omitempty"`
	CertManagerIssuer  string `json:"cert_manager_issuer,omitempty"`
	ImagePullSecret    string `json:"image_pull_secret,omitempty"`
	WorkspaceLBEnabled bool   `json:"workspace_lb_enabled,omitempty"`
	OrgID              string `json:"org_id,omitempty"`
}

// List returns all registered clusters
func (cc *ClusterClient) List(ctx context.Context) ([]Cluster, error) {
	var response struct {
		Clusters []Cluster `json:"clusters"`
	}

	err := cc.client.get(ctx, "/api/deploy/clusters", &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	return response.Clusters, nil
}

// Get returns details for a specific cluster
func (cc *ClusterClient) Get(ctx context.Context, id string) (*Cluster, error) {
	var cluster Cluster
	err := cc.client.get(ctx, fmt.Sprintf("/api/deploy/clusters/%s", id), &cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster %s: %w", id, err)
	}

	return &cluster, nil
}

// Bootstrap registers a new cluster with the provided kubeconfig
func (cc *ClusterClient) Bootstrap(ctx context.Context, kubeconfig []byte) (string, error) {
	payload := map[string]interface{}{
		"cluster": map[string]interface{}{
			"kubeconfig": base64.StdEncoding.EncodeToString(kubeconfig),
		},
	}

	var response struct {
		ClusterID string `json:"clusterId"`
	}

	err := cc.client.post(ctx, "/bootstrap", payload, &response)
	if err != nil {
		return "", fmt.Errorf("failed to bootstrap cluster: %w", err)
	}

	return response.ClusterID, nil
}

// UpdateSettings updates cluster-specific settings
func (cc *ClusterClient) UpdateSettings(ctx context.Context, id string, settings ClusterSettings) error {
	err := cc.client.put(ctx, fmt.Sprintf("/api/settings/cluster/%s", id), settings, nil)
	if err != nil {
		return fmt.Errorf("failed to update cluster settings: %w", err)
	}

	return nil
}

// GetSettings retrieves cluster settings
func (cc *ClusterClient) GetSettings(ctx context.Context, id string) (*ClusterSettings, error) {
	var settings ClusterSettings
	err := cc.client.get(ctx, fmt.Sprintf("/api/settings/cluster/%s", id), &settings)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster settings: %w", err)
	}

	return &settings, nil
}

// GetKubeconfig retrieves the stored kubeconfig for a cluster
func (cc *ClusterClient) GetKubeconfig(ctx context.Context, id string) ([]byte, error) {
	var response struct {
		Kubeconfig string `json:"kubeconfig"`
	}

	err := cc.client.post(ctx, fmt.Sprintf("/api/deploy/clusters/%s?action=kubeconfig", id), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	kubeconfig, err := base64.StdEncoding.DecodeString(response.Kubeconfig)
	if err != nil {
		// Try without base64 decoding in case it's plain text
		return []byte(response.Kubeconfig), nil
	}

	return kubeconfig, nil
}

// Delete removes a cluster registration
func (cc *ClusterClient) Delete(ctx context.Context, id string) error {
	err := cc.client.delete(ctx, fmt.Sprintf("/api/deploy/clusters/%s", id))
	if err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	return nil
}

// Health checks cluster health
func (cc *ClusterClient) Health(ctx context.Context, id string) (*ClusterHealth, error) {
	var health ClusterHealth
	err := cc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/health", id), &health)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster health: %w", err)
	}

	return &health, nil
}

// ClusterHealth represents cluster health status
type ClusterHealth struct {
	ClusterID         string `json:"clusterId"`
	KubeconfigPresent bool   `json:"kubeconfigPresent"`
	KubeconfigValid   bool   `json:"kubeconfigValid"`
	K8sReachable      bool   `json:"k8sReachable"`
	K8sError          string `json:"k8sError,omitempty"`
	PFAvailable       bool   `json:"pfAvailable"`
	TSAvailable       bool   `json:"tsnetAvailable"`
	RecommendedAction string `json:"recommendedAction,omitempty"`
}
