package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
	"github.com/docxology/GuildNet/metaguildnet/sdk/go/testing"
)

// TestRollingUpdate tests rolling update pattern
func TestRollingUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	apiURL := os.Getenv("GUILDNET_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	c := client.NewClient(apiURL, "")
	ctx := context.Background()

	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		t.Fatalf("Failed to list clusters: %v", err)
	}
	if len(clusters) == 0 {
		t.Fatal("No clusters available")
	}
	clusterID := clusters[0].ID

	workspaceName := "rolling-update-test"

	t.Run("DeployInitialVersion", func(t *testing.T) {
		spec := client.WorkspaceSpec{
			Name:  workspaceName,
			Image: "nginx:1.20",
			Labels: map[string]string{
				"test":    "true",
				"e2e":     "rolling-update",
				"version": "v1",
			},
		}

		_, err := c.Workspaces(clusterID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}

		err = testing.WaitForWorkspaceReady(c, clusterID, workspaceName, 5*time.Minute)
		if err != nil {
			t.Fatalf("Workspace did not become ready: %v", err)
		}

		t.Log("Initial version deployed")
	})

	t.Run("RollingUpdate", func(t *testing.T) {
		// Deploy new version with temporary name
		newName := workspaceName + "-new"

		spec := client.WorkspaceSpec{
			Name:  newName,
			Image: "nginx:1.21",
			Labels: map[string]string{
				"test":    "true",
				"e2e":     "rolling-update",
				"version": "v2",
			},
		}

		_, err := c.Workspaces(clusterID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create new version: %v", err)
		}

		// Wait for new version
		err = testing.WaitForWorkspaceReady(c, clusterID, newName, 5*time.Minute)
		if err != nil {
			// Rollback
			c.Workspaces(clusterID).Delete(ctx, newName)
			t.Fatalf("New version failed to become ready: %v", err)
		}

		t.Log("New version ready")

		// Verify both versions are running
		oldWs, err := c.Workspaces(clusterID).Get(ctx, workspaceName)
		if err != nil {
			t.Errorf("Failed to get old version: %v", err)
		} else if oldWs.Status != "Running" {
			t.Errorf("Old version not running: %s", oldWs.Status)
		}

		newWs, err := c.Workspaces(clusterID).Get(ctx, newName)
		if err != nil {
			t.Errorf("Failed to get new version: %v", err)
		} else if newWs.Status != "Running" {
			t.Errorf("New version not running: %s", newWs.Status)
		}

		// Switch over (delete old)
		err = c.Workspaces(clusterID).Delete(ctx, workspaceName)
		if err != nil {
			t.Errorf("Failed to delete old version: %v", err)
		}

		t.Log("Rolling update complete")
	})

	t.Run("Cleanup", func(t *testing.T) {
		c.Workspaces(clusterID).Delete(ctx, workspaceName)
		c.Workspaces(clusterID).Delete(ctx, workspaceName+"-new")
	})
}

// TestBlueGreenDeployment tests blue-green deployment pattern
func TestBlueGreenDeployment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	apiURL := os.Getenv("GUILDNET_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	c := client.NewClient(apiURL, "")
	ctx := context.Background()

	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		t.Fatalf("Failed to list clusters: %v", err)
	}
	if len(clusters) == 0 {
		t.Fatal("No clusters available")
	}
	clusterID := clusters[0].ID

	blueName := "bg-test-blue"
	greenName := "bg-test-green"

	t.Run("DeployBlue", func(t *testing.T) {
		spec := client.WorkspaceSpec{
			Name:  blueName,
			Image: "nginx:1.20",
			Labels: map[string]string{
				"test":       "true",
				"e2e":        "blue-green",
				"deployment": "blue",
			},
		}

		_, err := c.Workspaces(clusterID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create blue: %v", err)
		}

		err = testing.WaitForWorkspaceReady(c, clusterID, blueName, 5*time.Minute)
		if err != nil {
			t.Fatalf("Blue did not become ready: %v", err)
		}

		t.Log("Blue deployed")
	})

	t.Run("DeployGreen", func(t *testing.T) {
		spec := client.WorkspaceSpec{
			Name:  greenName,
			Image: "nginx:1.21",
			Labels: map[string]string{
				"test":       "true",
				"e2e":        "blue-green",
				"deployment": "green",
			},
		}

		_, err := c.Workspaces(clusterID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create green: %v", err)
		}

		err = testing.WaitForWorkspaceReady(c, clusterID, greenName, 5*time.Minute)
		if err != nil {
			// Rollback - keep blue
			c.Workspaces(clusterID).Delete(ctx, greenName)
			t.Fatalf("Green failed to become ready: %v", err)
		}

		t.Log("Green deployed")
	})

	t.Run("VerifyBothRunning", func(t *testing.T) {
		// Both should be running
		testing.AssertWorkspaceRunning(t, c, clusterID, blueName)
		testing.AssertWorkspaceRunning(t, c, clusterID, greenName)

		t.Log("Both blue and green are running")
	})

	t.Run("SwitchToGreen", func(t *testing.T) {
		// In real scenario, would update load balancer
		// For this test, just verify green is healthy and delete blue

		// Health check green
		green, err := c.Workspaces(clusterID).Get(ctx, greenName)
		if err != nil || green.Status != "Running" {
			t.Fatal("Green not healthy, aborting switch")
		}

		// Delete blue
		err = c.Workspaces(clusterID).Delete(ctx, blueName)
		if err != nil {
			t.Errorf("Failed to delete blue: %v", err)
		}

		t.Log("Switched to green")
	})

	t.Run("Cleanup", func(t *testing.T) {
		c.Workspaces(clusterID).Delete(ctx, blueName)
		c.Workspaces(clusterID).Delete(ctx, greenName)
	})
}

// TestCanaryDeployment tests canary deployment pattern
func TestCanaryDeployment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	apiURL := os.Getenv("GUILDNET_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	c := client.NewClient(apiURL, "")
	ctx := context.Background()

	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		t.Fatalf("Failed to list clusters: %v", err)
	}
	if len(clusters) == 0 {
		t.Fatal("No clusters available")
	}
	clusterID := clusters[0].ID

	stableName := "canary-test-stable"
	canaryName := "canary-test-canary"

	t.Run("DeployStable", func(t *testing.T) {
		spec := client.WorkspaceSpec{
			Name:  stableName,
			Image: "nginx:1.20",
			Labels: map[string]string{
				"test":       "true",
				"e2e":        "canary",
				"deployment": "stable",
			},
		}

		_, err := c.Workspaces(clusterID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create stable: %v", err)
		}

		err = testing.WaitForWorkspaceReady(c, clusterID, stableName, 5*time.Minute)
		if err != nil {
			t.Fatalf("Stable did not become ready: %v", err)
		}

		t.Log("Stable deployed")
	})

	t.Run("DeployCanary", func(t *testing.T) {
		spec := client.WorkspaceSpec{
			Name:  canaryName,
			Image: "nginx:1.21",
			Labels: map[string]string{
				"test":       "true",
				"e2e":        "canary",
				"deployment": "canary",
				"traffic":    "10",
			},
		}

		_, err := c.Workspaces(clusterID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create canary: %v", err)
		}

		err = testing.WaitForWorkspaceReady(c, clusterID, canaryName, 5*time.Minute)
		if err != nil {
			c.Workspaces(clusterID).Delete(ctx, canaryName)
			t.Fatalf("Canary failed to become ready: %v", err)
		}

		t.Log("Canary deployed")
	})

	t.Run("MonitorCanary", func(t *testing.T) {
		// Monitor canary for a short period
		monitorDuration := 30 * time.Second
		checkInterval := 5 * time.Second

		t.Logf("Monitoring canary for %s", monitorDuration)

		deadline := time.Now().Add(monitorDuration)
		for time.Now().Before(deadline) {
			canary, err := c.Workspaces(clusterID).Get(ctx, canaryName)
			if err != nil {
				t.Errorf("Failed to get canary: %v", err)
				break
			}

			if canary.Status != "Running" {
				t.Errorf("Canary unhealthy: %s", canary.Status)
				break
			}

			time.Sleep(checkInterval)
		}

		t.Log("Canary monitoring complete")
	})

	t.Run("PromoteCanary", func(t *testing.T) {
		// Canary is healthy, promote it
		// In real scenario, would gradually increase traffic
		// For this test, just delete stable and keep canary

		err := c.Workspaces(clusterID).Delete(ctx, stableName)
		if err != nil {
			t.Errorf("Failed to delete stable: %v", err)
		}

		t.Log("Canary promoted to stable")
	})

	t.Run("Cleanup", func(t *testing.T) {
		c.Workspaces(clusterID).Delete(ctx, stableName)
		c.Workspaces(clusterID).Delete(ctx, canaryName)
	})
}

// TestWorkspaceScaling tests scaling workspaces
func TestWorkspaceScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	apiURL := os.Getenv("GUILDNET_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	c := client.NewClient(apiURL, "")
	ctx := context.Background()

	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		t.Fatalf("Failed to list clusters: %v", err)
	}
	if len(clusters) == 0 {
		t.Fatal("No clusters available")
	}
	clusterID := clusters[0].ID

	baseWorkspaceName := "scale-test"

	t.Run("ScaleUp", func(t *testing.T) {
		// Create multiple instances
		numInstances := 5

		for i := 0; i < numInstances; i++ {
			name := fmt.Sprintf("%s-%d", baseWorkspaceName, i)

			spec := client.WorkspaceSpec{
				Name:  name,
				Image: "nginx:latest",
				Labels: map[string]string{
					"test":     "true",
					"e2e":      "scaling",
					"instance": fmt.Sprintf("%d", i),
				},
			}

			_, err := c.Workspaces(clusterID).Create(ctx, spec)
			if err != nil {
				t.Errorf("Failed to create instance %d: %v", i, err)
			}
		}

		t.Logf("Scaled up to %d instances", numInstances)
	})

	t.Run("VerifyScaleUp", func(t *testing.T) {
		// Wait for all instances
		numInstances := 5
		readyCount := 0

		for i := 0; i < numInstances; i++ {
			name := fmt.Sprintf("%s-%d", baseWorkspaceName, i)
			err := testing.WaitForWorkspaceReady(c, clusterID, name, 5*time.Minute)
			if err == nil {
				readyCount++
			}
		}

		t.Logf("%d/%d instances ready", readyCount, numInstances)

		if readyCount < numInstances/2 {
			t.Errorf("Too few instances ready: %d/%d", readyCount, numInstances)
		}
	})

	t.Run("ScaleDown", func(t *testing.T) {
		// Delete half the instances
		numInstances := 5

		for i := numInstances / 2; i < numInstances; i++ {
			name := fmt.Sprintf("%s-%d", baseWorkspaceName, i)
			err := c.Workspaces(clusterID).Delete(ctx, name)
			if err != nil {
				t.Errorf("Failed to delete instance %d: %v", i, err)
			}
		}

		t.Log("Scaled down")
	})

	t.Run("Cleanup", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			name := fmt.Sprintf("%s-%d", baseWorkspaceName, i)
			c.Workspaces(clusterID).Delete(ctx, name)
		}
	})
}
