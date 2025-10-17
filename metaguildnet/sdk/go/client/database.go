package client

import (
	"context"
	"fmt"

	"github.com/docxology/GuildNet/internal/model"
)

// DatabaseClient handles database operations for a specific cluster
type DatabaseClient struct {
	client    *Client
	clusterID string
}

// Database represents a database instance
type Database struct {
	ID            string `json:"id"`
	OrgID         string `json:"org_id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Replication   int    `json:"replication,omitempty"`
	Shards        int    `json:"shards,omitempty"`
	PrimaryRegion string `json:"primary_region,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// List returns all databases in the cluster
func (dc *DatabaseClient) List(ctx context.Context) ([]Database, error) {
	var response []Database

	err := dc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/db", dc.clusterID), &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	return response, nil
}

// Create creates a new database
func (dc *DatabaseClient) Create(ctx context.Context, name, description string) (*Database, error) {
	payload := map[string]interface{}{
		"name":        name,
		"description": description,
	}

	var db Database
	err := dc.client.post(ctx, fmt.Sprintf("/api/cluster/%s/db", dc.clusterID), payload, &db)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return &db, nil
}

// Get retrieves a specific database
func (dc *DatabaseClient) Get(ctx context.Context, dbID string) (*Database, error) {
	var db Database
	err := dc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/db/%s", dc.clusterID, dbID), &db)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	return &db, nil
}

// Delete deletes a database
func (dc *DatabaseClient) Delete(ctx context.Context, dbID string) error {
	err := dc.client.delete(ctx, fmt.Sprintf("/api/cluster/%s/db/%s", dc.clusterID, dbID))
	if err != nil {
		return fmt.Errorf("failed to delete database: %w", err)
	}

	return nil
}

// Tables returns all tables in a database
func (dc *DatabaseClient) Tables(ctx context.Context, dbID string) ([]model.Table, error) {
	var tables []model.Table

	err := dc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/db/%s/tables", dc.clusterID, dbID), &tables)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	return tables, nil
}

// CreateTable creates a new table with schema
func (dc *DatabaseClient) CreateTable(ctx context.Context, dbID string, table model.Table) error {
	err := dc.client.post(ctx, fmt.Sprintf("/api/cluster/%s/db/%s/tables", dc.clusterID, dbID), table, nil)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// DeleteTable deletes a table
func (dc *DatabaseClient) DeleteTable(ctx context.Context, dbID, table string) error {
	err := dc.client.delete(ctx, fmt.Sprintf("/api/cluster/%s/db/%s/tables/%s", dc.clusterID, dbID, table))
	if err != nil {
		return fmt.Errorf("failed to delete table: %w", err)
	}

	return nil
}

// Query queries rows from a table
func (dc *DatabaseClient) Query(ctx context.Context, dbID, table, orderBy string, limit int, cursor string, forward bool) ([]map[string]any, string, error) {
	path := fmt.Sprintf("/api/cluster/%s/db/%s/tables/%s/rows?limit=%d", dc.clusterID, dbID, table, limit)

	if orderBy != "" {
		path += "&orderBy=" + orderBy
	}
	if cursor != "" {
		path += "&cursor=" + cursor
	}
	if !forward {
		path += "&forward=false"
	}

	var response struct {
		Rows       []map[string]any `json:"rows"`
		NextCursor string           `json:"nextCursor"`
	}

	err := dc.client.get(ctx, path, &response)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query rows: %w", err)
	}

	return response.Rows, response.NextCursor, nil
}

// InsertRows inserts multiple rows into a table
func (dc *DatabaseClient) InsertRows(ctx context.Context, dbID, table string, rows []map[string]any) ([]string, error) {
	var response struct {
		IDs []string `json:"ids"`
	}

	err := dc.client.post(ctx, fmt.Sprintf("/api/cluster/%s/db/%s/tables/%s/rows", dc.clusterID, dbID, table), map[string]any{"rows": rows}, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to insert rows: %w", err)
	}

	return response.IDs, nil
}

// UpdateRow updates a single row
func (dc *DatabaseClient) UpdateRow(ctx context.Context, dbID, table, id string, patch map[string]any) error {
	err := dc.client.put(ctx, fmt.Sprintf("/api/cluster/%s/db/%s/tables/%s/rows/%s", dc.clusterID, dbID, table, id), patch, nil)
	if err != nil {
		return fmt.Errorf("failed to update row: %w", err)
	}

	return nil
}

// DeleteRow deletes a single row
func (dc *DatabaseClient) DeleteRow(ctx context.Context, dbID, table, id string) error {
	err := dc.client.delete(ctx, fmt.Sprintf("/api/cluster/%s/db/%s/tables/%s/rows/%s", dc.clusterID, dbID, table, id))
	if err != nil {
		return fmt.Errorf("failed to delete row: %w", err)
	}

	return nil
}

// GetRow retrieves a single row by ID
func (dc *DatabaseClient) GetRow(ctx context.Context, dbID, table, id string) (map[string]any, error) {
	var row map[string]any

	err := dc.client.get(ctx, fmt.Sprintf("/api/cluster/%s/db/%s/tables/%s/rows/%s", dc.clusterID, dbID, table, id), &row)
	if err != nil {
		return nil, fmt.Errorf("failed to get row: %w", err)
	}

	return row, nil
}

// ListAudit lists audit events for a database
func (dc *DatabaseClient) ListAudit(ctx context.Context, dbID string, limit int) ([]model.AuditEvent, error) {
	var events []model.AuditEvent

	path := fmt.Sprintf("/api/cluster/%s/db/%s/audit?limit=%d", dc.clusterID, dbID, limit)
	err := dc.client.get(ctx, path, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events: %w", err)
	}

	return events, nil
}
