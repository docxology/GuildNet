package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
	"github.com/docxology/GuildNet/metaguildnet/sdk/go/testing"
)

// TestFullWorkflow tests complete end-to-end workflow:
// Install -> Bootstrap -> Deploy -> Verify -> Cleanup
func TestFullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	// Setup
	apiURL := os.Getenv("GUILDNET_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	c := client.NewClient(apiURL, "")
	ctx := context.Background()

	t.Run("VerifyInstallation", func(t *testing.T) {
		// Verify GuildNet is installed and running
		health, err := c.Health().Global(ctx)
		if err != nil {
			t.Fatalf("Failed to get health: %v", err)
		}

		if health.Status != "healthy" {
			t.Errorf("GuildNet is not healthy: %s", health.Status)
		}
	})

	var clusterID string

	t.Run("BootstrapCluster", func(t *testing.T) {
		// Bootstrap should already be done in setup
		// We just verify we have at least one cluster
		clusters, err := c.Clusters().List(ctx)
		if err != nil {
			t.Fatalf("Failed to list clusters: %v", err)
		}

		if len(clusters) == 0 {
			t.Fatal("No clusters found, bootstrap may have failed")
		}

		clusterID = clusters[0].ID
		t.Logf("Using cluster: %s", clusterID)

		// Verify cluster is healthy
		testing.AssertClusterHealthy(t, c, clusterID)
	})

	workspaceName := "e2e-test-workspace"

	t.Run("DeployWorkspace", func(t *testing.T) {
		// Create a test workspace
		spec := client.WorkspaceSpec{
			Name:  workspaceName,
			Image: "nginx:latest",
			Labels: map[string]string{
				"test":  "true",
				"e2e":   "true",
				"suite": "full-workflow",
			},
		}

		ws, err := c.Workspaces(clusterID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}

		t.Logf("Created workspace: %s", ws.Name)

		// Wait for workspace to be ready
		err = testing.WaitForWorkspaceReady(c, clusterID, workspaceName, 5*time.Minute)
		if err != nil {
			t.Fatalf("Workspace did not become ready: %v", err)
		}

		// Verify workspace is running
		testing.AssertWorkspaceRunning(t, c, clusterID, workspaceName)
	})

	t.Run("VerifyWorkspace", func(t *testing.T) {
		// Get workspace details
		ws, err := c.Workspaces(clusterID).Get(ctx, workspaceName)
		if err != nil {
			t.Fatalf("Failed to get workspace: %v", err)
		}

		if ws.Status != "Running" {
			t.Errorf("Workspace not running: %s", ws.Status)
		}

		// Test logs access
		logs, err := c.Workspaces(clusterID).Logs(ctx, workspaceName)
		if err != nil {
			t.Errorf("Failed to get logs: %v", err)
		} else {
			t.Logf("Got %d log lines", len(logs))
		}
	})

	t.Run("TestDatabase", func(t *testing.T) {
		// List databases
		dbs, err := c.Databases(clusterID).List(ctx)
		if err != nil {
			t.Fatalf("Failed to list databases: %v", err)
		}

		t.Logf("Found %d databases", len(dbs))

		// Create test database
		testDB, err := c.Databases(clusterID).Create(ctx, "e2e_test_db", "E2E test database")
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}

		t.Logf("Created database: %s", testDB.Name)

		// Verify database exists
		dbs, err = c.Databases(clusterID).List(ctx)
		if err != nil {
			t.Fatalf("Failed to list databases: %v", err)
		}

		found := false
		for _, db := range dbs {
			if db.Name == "e2e_test_db" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Created database not found in list")
		}
	})

	t.Run("TestPublishedServices", func(t *testing.T) {
		// List published services
		published, err := c.Health().Published(ctx, clusterID)
		if err != nil {
			t.Fatalf("Failed to list published services: %v", err)
		}

		t.Logf("Found %d published services", len(published))

		// Our test workspace might not be published, that's OK
		// Just verify the API works
	})

	t.Run("Cleanup", func(t *testing.T) {
		// Delete test workspace
		err := c.Workspaces(clusterID).Delete(ctx, workspaceName)
		if err != nil {
			t.Errorf("Failed to delete workspace: %v", err)
		} else {
			t.Logf("Deleted workspace: %s", workspaceName)
		}

		// Wait a bit for cleanup
		time.Sleep(5 * time.Second)

		// Verify workspace is gone
		_, err = c.Workspaces(clusterID).Get(ctx, workspaceName)
		if err == nil {
			t.Error("Workspace still exists after deletion")
		}
	})
}

// TestWorkspaceLifecycle tests workspace lifecycle management
func TestWorkspaceLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	apiURL := os.Getenv("GUILDNET_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	c := client.NewClient(apiURL, "")
	ctx := context.Background()

	// Get first cluster
	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		t.Fatalf("Failed to list clusters: %v", err)
	}
	if len(clusters) == 0 {
		t.Fatal("No clusters available")
	}
	clusterID := clusters[0].ID

	workspaces := []string{
		"lifecycle-test-1",
		"lifecycle-test-2",
		"lifecycle-test-3",
	}

	// Create multiple workspaces
	t.Run("CreateMultiple", func(t *testing.T) {
		for _, name := range workspaces {
			spec := client.WorkspaceSpec{
				Name:  name,
				Image: "nginx:latest",
				Labels: map[string]string{
					"test": "true",
					"e2e":  "lifecycle",
				},
			}

			_, err := c.Workspaces(clusterID).Create(ctx, spec)
			if err != nil {
				t.Errorf("Failed to create workspace %s: %v", name, err)
			}
		}
	})

	// Wait for all to be ready
	t.Run("WaitForAll", func(t *testing.T) {
		for _, name := range workspaces {
			err := testing.WaitForWorkspaceReady(c, clusterID, name, 5*time.Minute)
			if err != nil {
				t.Errorf("Workspace %s did not become ready: %v", name, err)
			}
		}
	})

	// Update one workspace
	t.Run("UpdateWorkspace", func(t *testing.T) {
		// In real scenario, would update the workspace
		// For now, just verify we can get and check it
		ws, err := c.Workspaces(clusterID).Get(ctx, workspaces[0])
		if err != nil {
			t.Errorf("Failed to get workspace: %v", err)
		} else {
			t.Logf("Workspace %s status: %s", ws.Name, ws.Status)
		}
	})

	// Clean up
	t.Run("CleanupAll", func(t *testing.T) {
		for _, name := range workspaces {
			err := c.Workspaces(clusterID).Delete(ctx, name)
			if err != nil {
				t.Errorf("Failed to delete workspace %s: %v", name, err)
			}
		}
	})
}
