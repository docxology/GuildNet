package client

import (
	"context"
	"fmt"
	"time"
)

// HealthClient handles health and status operations
type HealthClient struct {
	client *Client
}

// HealthSummary represents overall system health
type HealthSummary struct {
	Healthy     bool              `json:"healthy"`
	Headscale   []HeadscaleStatus `json:"headscale,omitempty"`
	Clusters    []ClusterHealth   `json:"clusters"`
	LastChecked time.Time         `json:"lastChecked"`
}

// HeadscaleStatus represents Headscale health
type HeadscaleStatus struct {
	ID      string `json:"id"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

// PublishedService represents a published tsnet service
type PublishedService struct {
	ClusterID string    `json:"cluster_id"`
	Service   string    `json:"service"`
	Addr      string    `json:"addr"`
	AddedAt   time.Time `json:"added_at"`
}

// Global returns overall system health
func (hc *HealthClient) Global(ctx context.Context) (*HealthSummary, error) {
	var health HealthSummary

	err := hc.client.get(ctx, "/api/health", &health)
	if err != nil {
		return nil, fmt.Errorf("failed to get global health: %w", err)
	}

	health.LastChecked = time.Now()
	return &health, nil
}

// Cluster returns health status for a specific cluster
func (hc *HealthClient) Cluster(ctx context.Context, id string) (*ClusterHealth, error) {
	var health ClusterHealth

	err := hc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/health", id), &health)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster health: %w", err)
	}

	return &health, nil
}

// Published returns all published services for a cluster
func (hc *HealthClient) Published(ctx context.Context, clusterID string) ([]PublishedService, error) {
	var services []PublishedService

	err := hc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/published-services", clusterID), &services)
	if err != nil {
		return nil, fmt.Errorf("failed to get published services: %w", err)
	}

	return services, nil
}

// DeletePublished removes a published service
func (hc *HealthClient) DeletePublished(ctx context.Context, clusterID, service string) error {
	err := hc.client.delete(ctx, fmt.Sprintf("/api/cluster/%s/published-services/%s", clusterID, service))
	if err != nil {
		return fmt.Errorf("failed to delete published service: %w", err)
	}

	return nil
}

// Status returns a quick health check (simpler than Global)
func (hc *HealthClient) Status(ctx context.Context) (bool, error) {
	err := hc.client.get(ctx, "/healthz", nil)
	return err == nil, err
}
