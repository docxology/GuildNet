package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
)

// This example demonstrates a basic workflow:
// 1. Connect to GuildNet
// 2. List clusters
// 3. Create a workspace
// 4. Wait for it to be ready
// 5. Get logs
// 6. Clean up

func main() {
	// Get configuration from environment
	apiURL := os.Getenv("MGN_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	token := os.Getenv("MGN_API_TOKEN")

	// Create client
	c := client.NewClient(apiURL, token,
		client.WithTimeout(30*time.Second),
		client.WithMaxRetries(3))

	ctx := context.Background()

	// List clusters
	fmt.Println("Listing clusters...")
	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		log.Fatalf("Failed to list clusters: %v", err)
	}

	if len(clusters) == 0 {
		log.Fatal("No clusters available")
	}

	clusterID := clusters[0].ID
	fmt.Printf("Using cluster: %s (%s)\n", clusters[0].Name, clusterID)

	// Check cluster health
	fmt.Println("\nChecking cluster health...")
	health, err := c.Health().Cluster(ctx, clusterID)
	if err != nil {
		log.Fatalf("Failed to get cluster health: %v", err)
	}

	fmt.Printf("  K8s Reachable: %v\n", health.K8sReachable)
	fmt.Printf("  Kubeconfig Valid: %v\n", health.KubeconfigValid)

	if !health.K8sReachable {
		log.Fatal("Cluster is not healthy")
	}

	// Create workspace
	workspaceName := fmt.Sprintf("example-%d", time.Now().Unix())
	fmt.Printf("\nCreating workspace: %s\n", workspaceName)

	spec := client.WorkspaceSpec{
		Name:  workspaceName,
		Image: "nginx:alpine",
		Labels: map[string]string{
			"example": "basic-workflow",
		},
	}

	ws, err := c.Workspaces(clusterID).Create(ctx, spec)
	if err != nil {
		log.Fatalf("Failed to create workspace: %v", err)
	}

	fmt.Printf("Workspace created: %s (ID: %s)\n", ws.Name, ws.ID)

	// Wait for workspace to be ready
	fmt.Println("\nWaiting for workspace to be ready...")
	err = c.Workspaces(clusterID).Wait(ctx, workspaceName, 5*time.Minute)
	if err != nil {
		log.Fatalf("Workspace failed to start: %v", err)
	}

	fmt.Println("Workspace is ready!")

	// Get workspace details
	ws, err = c.Workspaces(clusterID).Get(ctx, workspaceName)
	if err != nil {
		log.Fatalf("Failed to get workspace: %v", err)
	}

	fmt.Printf("\nWorkspace details:\n")
	fmt.Printf("  Status: %s\n", ws.Status)
	fmt.Printf("  Service DNS: %s\n", ws.ServiceDNS)
	fmt.Printf("  Service IP: %s\n", ws.ServiceIP)
	if ws.ExternalURL != "" {
		fmt.Printf("  External URL: %s\n", ws.ExternalURL)
	}

	// Get logs
	fmt.Println("\nFetching logs...")
	logs, err := c.Workspaces(clusterID).Logs(ctx, workspaceName, client.LogOptions{
		TailLines: 20,
	})
	if err != nil {
		log.Printf("Warning: failed to get logs: %v", err)
	} else {
		for _, line := range logs {
			fmt.Printf("  [%s] %s\n", line.Timestamp.Format("15:04:05"), line.Line)
		}
	}

	// Get proxy URL
	proxyURL := c.Workspaces(clusterID).ProxyURL(workspaceName)
	fmt.Printf("\nProxy URL: %s\n", proxyURL)

	// Clean up
	fmt.Println("\nCleaning up...")
	err = c.Workspaces(clusterID).Delete(ctx, workspaceName)
	if err != nil {
		log.Fatalf("Failed to delete workspace: %v", err)
	}

	fmt.Println("Workspace deleted successfully")
	fmt.Println("\nBasic workflow complete!")
}
