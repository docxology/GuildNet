package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
)

// This example demonstrates multi-cluster operations:
// 1. List all clusters
// 2. Deploy the same workspace to multiple clusters
// 3. Monitor health across clusters
// 4. Clean up

func main() {
	apiURL := os.Getenv("MGN_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	token := os.Getenv("MGN_API_TOKEN")

	c := client.NewClient(apiURL, token)
	ctx := context.Background()

	// List all clusters
	fmt.Println("Discovering clusters...")
	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		log.Fatalf("Failed to list clusters: %v", err)
	}

	fmt.Printf("Found %d cluster(s):\n", len(clusters))
	for i, cluster := range clusters {
		fmt.Printf("  %d. %s (%s)\n", i+1, cluster.Name, cluster.ID)
	}

	if len(clusters) == 0 {
		log.Fatal("No clusters available")
	}

	// Check health of all clusters
	fmt.Println("\nChecking cluster health...")
	healthyClusters := []client.Cluster{}

	for _, cluster := range clusters {
		health, err := c.Health().Cluster(ctx, cluster.ID)
		if err != nil {
			fmt.Printf("  ✗ %s: failed to check health: %v\n", cluster.Name, err)
			continue
		}

		if health.K8sReachable {
			fmt.Printf("  ✓ %s: healthy\n", cluster.Name)
			healthyClusters = append(healthyClusters, cluster)
		} else {
			fmt.Printf("  ✗ %s: unhealthy (%s)\n", cluster.Name, health.K8sError)
		}
	}

	if len(healthyClusters) == 0 {
		log.Fatal("No healthy clusters available")
	}

	// Deploy workspace to all healthy clusters
	workspaceName := fmt.Sprintf("multi-cluster-%d", time.Now().Unix())
	spec := client.WorkspaceSpec{
		Name:  workspaceName,
		Image: "nginx:alpine",
		Labels: map[string]string{
			"example":     "multi-cluster",
			"deployed-at": time.Now().Format(time.RFC3339),
		},
	}

	fmt.Printf("\nDeploying workspace '%s' to %d cluster(s)...\n", workspaceName, len(healthyClusters))

	var wg sync.WaitGroup
	results := make(chan deployResult, len(healthyClusters))

	for _, cluster := range healthyClusters {
		wg.Add(1)
		go func(cl client.Cluster) {
			defer wg.Done()
			deployToCluster(c, ctx, cl, spec, results)
		}(cluster)
	}

	wg.Wait()
	close(results)

	// Collect results
	successful := []deployResult{}
	failed := []deployResult{}

	for result := range results {
		if result.err == nil {
			successful = append(successful, result)
		} else {
			failed = append(failed, result)
		}
	}

	// Report results
	fmt.Printf("\nDeployment complete:\n")
	fmt.Printf("  Successful: %d\n", len(successful))
	fmt.Printf("  Failed: %d\n", len(failed))

	if len(successful) > 0 {
		fmt.Println("\nSuccessful deployments:")
		for _, result := range successful {
			fmt.Printf("  ✓ %s: %s\n", result.clusterName, result.status)
		}
	}

	if len(failed) > 0 {
		fmt.Println("\nFailed deployments:")
		for _, result := range failed {
			fmt.Printf("  ✗ %s: %v\n", result.clusterName, result.err)
		}
	}

	// Monitor workspace status across clusters
	fmt.Println("\nMonitoring workspace status...")
	time.Sleep(5 * time.Second)

	for _, result := range successful {
		ws, err := c.Workspaces(result.clusterID).Get(ctx, workspaceName)
		if err != nil {
			fmt.Printf("  %s: failed to get status: %v\n", result.clusterName, err)
			continue
		}

		fmt.Printf("  %s: %s (ready: %d)\n", result.clusterName, ws.Status, ws.ReadyReplicas)
	}

	// Clean up
	fmt.Println("\nCleaning up...")
	for _, result := range successful {
		err := c.Workspaces(result.clusterID).Delete(ctx, workspaceName)
		if err != nil {
			fmt.Printf("  Failed to delete from %s: %v\n", result.clusterName, err)
		} else {
			fmt.Printf("  Deleted from %s\n", result.clusterName)
		}
	}

	fmt.Println("\nMulti-cluster example complete!")
}

type deployResult struct {
	clusterID   string
	clusterName string
	status      string
	err         error
}

func deployToCluster(c *client.Client, ctx context.Context, cluster client.Cluster, spec client.WorkspaceSpec, results chan<- deployResult) {
	result := deployResult{
		clusterID:   cluster.ID,
		clusterName: cluster.Name,
	}

	// Create workspace
	ws, err := c.Workspaces(cluster.ID).Create(ctx, spec)
	if err != nil {
		result.err = fmt.Errorf("create failed: %w", err)
		results <- result
		return
	}

	// Wait for ready (with timeout)
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	err = c.Workspaces(cluster.ID).Wait(waitCtx, spec.Name, 3*time.Minute)
	if err != nil {
		result.err = fmt.Errorf("wait failed: %w", err)
		results <- result
		return
	}

	result.status = ws.Status
	results <- result
}
