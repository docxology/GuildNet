package integration

import (
	"context"
	"testing"
	"time"

	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
	mgntesting "github.com/docxology/GuildNet/metaguildnet/sdk/go/testing"
)

func TestGoSDKIntegration(t *testing.T) {
	// Create client
	c := client.NewClient("https://localhost:8090", "")

	// Create test cluster
	tc := mgntesting.NewTestCluster(t)
	defer tc.Cleanup()

	ctx := context.Background()

	t.Run("ListClusters", func(t *testing.T) {
		clusters, err := c.Clusters().List(ctx)
		mgntesting.AssertNoError(t, err, "failed to list clusters")
		mgntesting.AssertGreaterThan(t, len(clusters), 0, "expected at least one cluster")
	})

	t.Run("ClusterHealth", func(t *testing.T) {
		mgntesting.AssertClusterHealthy(t, c, tc.ID)
	})

	t.Run("CreateWorkspace", func(t *testing.T) {
		spec := client.WorkspaceSpec{
			Name:  "test-integration",
			Image: "nginx:alpine",
		}

		ws, err := c.Workspaces(tc.ID).Create(ctx, spec)
		mgntesting.AssertNoError(t, err, "failed to create workspace")

		// Cleanup
		defer c.Workspaces(tc.ID).Delete(ctx, ws.Name)

		// Wait for ready
		err = c.Workspaces(tc.ID).Wait(ctx, ws.Name, 2*time.Minute)
		mgntesting.AssertNoError(t, err, "workspace did not become ready")

		// Assert running
		mgntesting.AssertWorkspaceRunning(t, c, tc.ID, ws.Name)
	})

	t.Run("WorkspaceLifecycle", func(t *testing.T) {
		// Create
		ws, cleanup := mgntesting.CreateTestWorkspace(t, c, tc.ID)
		defer cleanup()

		// Get
		fetched, err := c.Workspaces(tc.ID).Get(ctx, ws.Name)
		mgntesting.AssertNoError(t, err, "failed to get workspace")
		mgntesting.AssertEqual(t, ws.Name, fetched.Name, "workspace names don't match")

		// Delete
		err = c.Workspaces(tc.ID).Delete(ctx, ws.Name)
		mgntesting.AssertNoError(t, err, "failed to delete workspace")

		// Verify deleted
		mgntesting.AssertEventually(t, func() bool {
			_, err := c.Workspaces(tc.ID).Get(ctx, ws.Name)
			return err != nil
		}, 30*time.Second, "workspace was not deleted")
	})
}

func TestDatabaseOperations(t *testing.T) {
	c := client.NewClient("https://localhost:8090", "")
	tc := mgntesting.NewTestCluster(t)
	defer tc.Cleanup()

	ctx := context.Background()

	t.Run("CreateDatabase", func(t *testing.T) {
		db, err := c.Databases(tc.ID).Create(ctx, "test_db", "Integration test database")
		mgntesting.AssertNoError(t, err, "failed to create database")

		defer c.Databases(tc.ID).Delete(ctx, db.ID)

		mgntesting.AssertDatabaseExists(t, c, tc.ID, db.ID)
	})
}
