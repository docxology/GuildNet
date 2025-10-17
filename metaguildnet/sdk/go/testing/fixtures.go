package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
)

// TestCluster represents a test cluster with cleanup
type TestCluster struct {
	ID      string
	Client  *client.Client
	Cleanup func()
	t       *testing.T
}

// NewTestCluster creates a test cluster that auto-cleans up
func NewTestCluster(t *testing.T) *TestCluster {
	t.Helper()

	// Create client
	c := client.NewClient("https://localhost:8090", "")

	// Use first available cluster (or create one if needed)
	ctx := context.Background()
	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		t.Fatalf("failed to list clusters: %v", err)
	}

	if len(clusters) == 0 {
		t.Fatal("no clusters available for testing")
	}

	clusterID := clusters[0].ID

	tc := &TestCluster{
		ID:     clusterID,
		Client: c,
		t:      t,
	}

	// Setup cleanup to delete test resources
	tc.Cleanup = func() {
		// Delete all test workspaces created during this test
		// (identified by labels or naming convention)
		ctx := context.Background()
		workspaces, err := c.Workspaces(clusterID).List(ctx)
		if err != nil {
			t.Logf("cleanup: failed to list workspaces: %v", err)
			return
		}

		for _, ws := range workspaces {
			// Only delete workspaces that look like test workspaces
			if len(ws.Name) > 5 && ws.Name[:5] == "test-" {
				err := c.Workspaces(clusterID).Delete(ctx, ws.Name)
				if err != nil {
					t.Logf("cleanup: failed to delete workspace %s: %v", ws.Name, err)
				}
			}
		}
	}

	return tc
}

// MockClient returns a mock client for testing without a real GuildNet instance
type MockClient struct {
	clusters   []client.Cluster
	workspaces map[string][]client.Workspace
}

// NewMockClient creates a mock client
func NewMockClient() *MockClient {
	return &MockClient{
		clusters: []client.Cluster{
			{
				ID:   "test-cluster-1",
				Name: "Test Cluster 1",
			},
		},
		workspaces: make(map[string][]client.Workspace),
	}
}

// Clusters returns a mock cluster client
func (mc *MockClient) Clusters() *MockClusterClient {
	return &MockClusterClient{mock: mc}
}

// Workspaces returns a mock workspace client
func (mc *MockClient) Workspaces(clusterID string) *MockWorkspaceClient {
	return &MockWorkspaceClient{mock: mc, clusterID: clusterID}
}

// MockClusterClient mocks cluster operations
type MockClusterClient struct {
	mock *MockClient
}

// List returns mock clusters
func (mcc *MockClusterClient) List(ctx context.Context) ([]client.Cluster, error) {
	return mcc.mock.clusters, nil
}

// Get returns a mock cluster
func (mcc *MockClusterClient) Get(ctx context.Context, id string) (*client.Cluster, error) {
	for _, c := range mcc.mock.clusters {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, client.ErrNotFound
}

// MockWorkspaceClient mocks workspace operations
type MockWorkspaceClient struct {
	mock      *MockClient
	clusterID string
}

// List returns mock workspaces
func (mwc *MockWorkspaceClient) List(ctx context.Context) ([]client.Workspace, error) {
	if ws, ok := mwc.mock.workspaces[mwc.clusterID]; ok {
		return ws, nil
	}
	return []client.Workspace{}, nil
}

// Create creates a mock workspace
func (mwc *MockWorkspaceClient) Create(ctx context.Context, spec client.WorkspaceSpec) (*client.Workspace, error) {
	ws := &client.Workspace{
		ID:     fmt.Sprintf("ws-%d", time.Now().Unix()),
		Name:   spec.Name,
		Image:  spec.Image,
		Status: "Running",
	}

	if mwc.mock.workspaces[mwc.clusterID] == nil {
		mwc.mock.workspaces[mwc.clusterID] = []client.Workspace{}
	}
	mwc.mock.workspaces[mwc.clusterID] = append(mwc.mock.workspaces[mwc.clusterID], *ws)

	return ws, nil
}

// Get returns a mock workspace
func (mwc *MockWorkspaceClient) Get(ctx context.Context, name string) (*client.Workspace, error) {
	workspaces, ok := mwc.mock.workspaces[mwc.clusterID]
	if !ok {
		return nil, client.ErrNotFound
	}

	for _, ws := range workspaces {
		if ws.Name == name {
			return &ws, nil
		}
	}
	return nil, client.ErrNotFound
}

// Delete deletes a mock workspace
func (mwc *MockWorkspaceClient) Delete(ctx context.Context, name string) error {
	workspaces, ok := mwc.mock.workspaces[mwc.clusterID]
	if !ok {
		return client.ErrNotFound
	}

	for i, ws := range workspaces {
		if ws.Name == name {
			mwc.mock.workspaces[mwc.clusterID] = append(workspaces[:i], workspaces[i+1:]...)
			return nil
		}
	}
	return client.ErrNotFound
}

// WaitForWorkspaceReady waits for a workspace to reach Running status
func WaitForWorkspaceReady(c *client.Client, clusterID, name string, timeout time.Duration) error {
	return c.Workspaces(clusterID).Wait(context.Background(), name, timeout)
}

// CreateTestWorkspace creates a workspace and returns cleanup function
func CreateTestWorkspace(t *testing.T, c *client.Client, clusterID string) (*client.Workspace, func()) {
	t.Helper()

	ctx := context.Background()

	spec := client.WorkspaceSpec{
		Name:  fmt.Sprintf("test-%d", time.Now().Unix()),
		Image: "nginx:alpine",
	}

	ws, err := c.Workspaces(clusterID).Create(ctx, spec)
	if err != nil {
		t.Fatalf("failed to create test workspace: %v", err)
	}

	cleanup := func() {
		ctx := context.Background()
		err := c.Workspaces(clusterID).Delete(ctx, ws.Name)
		if err != nil {
			t.Logf("cleanup: failed to delete workspace: %v", err)
		}
	}

	return ws, cleanup
}
