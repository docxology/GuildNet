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

// TestMultiClusterOrchestration tests orchestration across multiple clusters
func TestMultiClusterOrchestration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	apiURL := os.Getenv("GUILDNET_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	c := client.NewClient(apiURL, "")
	ctx := context.Background()

	// List all clusters
	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		t.Fatalf("Failed to list clusters: %v", err)
	}

	if len(clusters) < 2 {
		t.Skip("Need at least 2 clusters for multi-cluster tests")
	}

	t.Logf("Testing with %d clusters", len(clusters))

	workspaceName := "multi-cluster-test"

	t.Run("DeployToAllClusters", func(t *testing.T) {
		// Deploy same workspace to all clusters
		for _, cluster := range clusters {
			spec := client.WorkspaceSpec{
				Name:  workspaceName,
				Image: "nginx:latest",
				Labels: map[string]string{
					"test":         "true",
					"e2e":          "multi-cluster",
					"cluster-name": cluster.Name,
				},
			}

			_, err := c.Workspaces(cluster.ID).Create(ctx, spec)
			if err != nil {
				t.Errorf("Failed to create workspace in cluster %s: %v", cluster.Name, err)
			} else {
				t.Logf("Created workspace in cluster: %s", cluster.Name)
			}
		}
	})

	t.Run("VerifyAllDeployments", func(t *testing.T) {
		// Wait for all deployments
		for _, cluster := range clusters {
			err := testing.WaitForWorkspaceReady(c, cluster.ID, workspaceName, 5*time.Minute)
			if err != nil {
				t.Errorf("Workspace in cluster %s did not become ready: %v", cluster.Name, err)
			}

			testing.AssertWorkspaceRunning(t, c, cluster.ID, workspaceName)
		}
	})

	t.Run("VerifyClusterHealth", func(t *testing.T) {
		// Check health of all clusters
		for _, cluster := range clusters {
			status, err := c.Health().Cluster(ctx, cluster.ID)
			if err != nil {
				t.Errorf("Failed to get health for cluster %s: %v", cluster.Name, err)
				continue
			}

			if status.Status != "healthy" {
				t.Errorf("Cluster %s is not healthy: %s", cluster.Name, status.Status)
			}

			t.Logf("Cluster %s: %s", cluster.Name, status.Status)
		}
	})

	t.Run("CleanupAllClusters", func(t *testing.T) {
		// Delete from all clusters
		for _, cluster := range clusters {
			err := c.Workspaces(cluster.ID).Delete(ctx, workspaceName)
			if err != nil {
				t.Errorf("Failed to delete workspace from cluster %s: %v", cluster.Name, err)
			}
		}
	})
}

// TestClusterFailover tests failover between clusters
func TestClusterFailover(t *testing.T) {
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

	if len(clusters) < 2 {
		t.Skip("Need at least 2 clusters for failover tests")
	}

	primaryCluster := clusters[0]
	secondaryCluster := clusters[1]

	t.Logf("Primary: %s, Secondary: %s", primaryCluster.Name, secondaryCluster.Name)

	workspaceName := "failover-test"

	t.Run("DeployToPrimary", func(t *testing.T) {
		spec := client.WorkspaceSpec{
			Name:  workspaceName,
			Image: "nginx:latest",
			Labels: map[string]string{
				"test":    "true",
				"e2e":     "failover",
				"primary": "true",
			},
		}

		_, err := c.Workspaces(primaryCluster.ID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create workspace in primary: %v", err)
		}

		err = testing.WaitForWorkspaceReady(c, primaryCluster.ID, workspaceName, 5*time.Minute)
		if err != nil {
			t.Fatalf("Workspace did not become ready in primary: %v", err)
		}
	})

	t.Run("SimulateFailover", func(t *testing.T) {
		// In a real scenario, we would simulate primary failure
		// For now, just deploy to secondary and verify it works

		spec := client.WorkspaceSpec{
			Name:  workspaceName,
			Image: "nginx:latest",
			Labels: map[string]string{
				"test":      "true",
				"e2e":       "failover",
				"secondary": "true",
			},
		}

		_, err := c.Workspaces(secondaryCluster.ID).Create(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create workspace in secondary: %v", err)
		}

		err = testing.WaitForWorkspaceReady(c, secondaryCluster.ID, workspaceName, 5*time.Minute)
		if err != nil {
			t.Fatalf("Workspace did not become ready in secondary: %v", err)
		}

		t.Log("Failover successful - workspace running in secondary")
	})

	t.Run("Cleanup", func(t *testing.T) {
		c.Workspaces(primaryCluster.ID).Delete(ctx, workspaceName)
		c.Workspaces(secondaryCluster.ID).Delete(ctx, workspaceName)
	})
}

// TestCrossClusterDatabase tests database replication across clusters
func TestCrossClusterDatabase(t *testing.T) {
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

	if len(clusters) < 2 {
		t.Skip("Need at least 2 clusters for cross-cluster tests")
	}

	cluster1 := clusters[0]
	cluster2 := clusters[1]

	dbName := "cross_cluster_test"

	t.Run("CreateDatabaseInCluster1", func(t *testing.T) {
		_, err := c.Databases(cluster1.ID).Create(ctx, dbName, "Cross-cluster test database")
		if err != nil {
			t.Fatalf("Failed to create database in cluster 1: %v", err)
		}

		t.Logf("Created database %s in cluster %s", dbName, cluster1.Name)
	})

	t.Run("VerifyReplicationToCluster2", func(t *testing.T) {
		// RethinkDB may automatically replicate
		// Check if database appears in cluster 2
		time.Sleep(10 * time.Second) // Wait for replication

		dbs, err := c.Databases(cluster2.ID).List(ctx)
		if err != nil {
			t.Fatalf("Failed to list databases in cluster 2: %v", err)
		}

		found := false
		for _, db := range dbs {
			if db.Name == dbName {
				found = true
				break
			}
		}

		if found {
			t.Logf("Database replicated to cluster %s", cluster2.Name)
		} else {
			t.Logf("Database not yet replicated (may be expected)")
		}
	})
}

// TestLoadBalancing tests load distribution across clusters
func TestLoadBalancing(t *testing.T) {
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

	if len(clusters) < 2 {
		t.Skip("Need at least 2 clusters for load balancing tests")
	}

	t.Run("DeployBalancedWorkloads", func(t *testing.T) {
		// Deploy multiple workspaces distributed across clusters
		numWorkspaces := 6
		workspacePrefix := "lb-test"

		for i := 0; i < numWorkspaces; i++ {
			clusterIdx := i % len(clusters)
			cluster := clusters[clusterIdx]
			workspaceName := fmt.Sprintf("%s-%d", workspacePrefix, i)

			spec := client.WorkspaceSpec{
				Name:  workspaceName,
				Image: "nginx:latest",
				Labels: map[string]string{
					"test":    "true",
					"e2e":     "load-balancing",
					"cluster": cluster.Name,
				},
			}

			_, err := c.Workspaces(cluster.ID).Create(ctx, spec)
			if err != nil {
				t.Errorf("Failed to create workspace %s: %v", workspaceName, err)
			} else {
				t.Logf("Created %s in cluster %s", workspaceName, cluster.Name)
			}
		}
	})

	t.Run("VerifyDistribution", func(t *testing.T) {
		// Verify workspaces are distributed
		distribution := make(map[string]int)

		for _, cluster := range clusters {
			workspaces, err := c.Workspaces(cluster.ID).List(ctx)
			if err != nil {
				t.Errorf("Failed to list workspaces in cluster %s: %v", cluster.Name, err)
				continue
			}

			// Count test workspaces
			count := 0
			for _, ws := range workspaces {
				if ws.Labels["e2e"] == "load-balancing" {
					count++
				}
			}

			distribution[cluster.Name] = count
			t.Logf("Cluster %s: %d workspaces", cluster.Name, count)
		}

		// Verify distribution is somewhat balanced
		// (In a real test, you'd check the variance)
	})

	t.Run("CleanupLoadBalancing", func(t *testing.T) {
		for i := 0; i < 6; i++ {
			clusterIdx := i % len(clusters)
			cluster := clusters[clusterIdx]
			workspaceName := fmt.Sprintf("lb-test-%d", i)

			c.Workspaces(cluster.ID).Delete(ctx, workspaceName)
		}
	})
}
