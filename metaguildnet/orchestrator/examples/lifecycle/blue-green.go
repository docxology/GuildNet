package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
)

// Blue-Green deployment example
// Deploys new version alongside old, tests it, then switches traffic

func main() {
	clusterID := flag.String("cluster", "", "Cluster ID")
	workspaceName := flag.String("workspace", "", "Workspace name")
	newImage := flag.String("new-image", "", "New image to deploy")
	flag.Parse()

	if *clusterID == "" || *workspaceName == "" || *newImage == "" {
		log.Fatal("Usage: blue-green --cluster <id> --workspace <name> --new-image <image>")
	}

	c := client.NewClient("https://localhost:8090", "")
	ctx := context.Background()

	fmt.Println("Blue-Green Deployment")
	fmt.Printf("Cluster: %s\n", *clusterID)
	fmt.Printf("Workspace: %s\n", *workspaceName)
	fmt.Printf("New Image: %s\n", *newImage)
	fmt.Println()

	// Get current workspace (blue)
	fmt.Println("Fetching current workspace (blue)...")
	blue, err := c.Workspaces(*clusterID).Get(ctx, *workspaceName)
	if err != nil {
		log.Fatalf("Failed to get workspace: %v", err)
	}

	fmt.Printf("Current version: %s\n", blue.Image)
	fmt.Println()

	// Deploy new version (green)
	greenName := *workspaceName + "-green"
	fmt.Printf("Deploying new version (green): %s\n", greenName)

	greenSpec := client.WorkspaceSpec{
		Name:  greenName,
		Image: *newImage,
		Labels: map[string]string{
			"deployment": "green",
			"parent":     *workspaceName,
		},
	}

	_, err = c.Workspaces(*clusterID).Create(ctx, greenSpec)
	if err != nil {
		log.Fatalf("Failed to create green deployment: %v", err)
	}

	// Wait for green to be ready
	fmt.Println("Waiting for green deployment to be ready...")
	err = c.Workspaces(*clusterID).Wait(ctx, greenName, 5*time.Minute)
	if err != nil {
		log.Printf("Green deployment failed: %v", err)
		log.Println("Rolling back...")
		c.Workspaces(*clusterID).Delete(ctx, greenName)
		log.Fatal("Deployment failed")
	}

	fmt.Println("✓ Green deployment is ready")
	fmt.Println()

	// Run health checks on green
	fmt.Println("Running health checks on green deployment...")
	time.Sleep(5 * time.Second)

	greenStatus, err := c.Workspaces(*clusterID).Get(ctx, greenName)
	if err != nil || greenStatus.Status != "Running" {
		log.Println("Health check failed")
		log.Println("Rolling back...")
		c.Workspaces(*clusterID).Delete(ctx, greenName)
		log.Fatal("Health check failed")
	}

	fmt.Println("✓ Health checks passed")
	fmt.Println()

	// Prompt for traffic switch
	fmt.Println("Green deployment is healthy and ready for traffic")
	fmt.Printf("Blue URL: %s\n", c.Workspaces(*clusterID).ProxyURL(*workspaceName))
	fmt.Printf("Green URL: %s\n", c.Workspaces(*clusterID).ProxyURL(greenName))
	fmt.Println()

	// In a real scenario, you would:
	// 1. Update load balancer / ingress to point to green
	// 2. Monitor green for issues
	// 3. If successful, delete blue
	// 4. If issues, switch back to blue

	fmt.Println("To complete the switch:")
	fmt.Println("  1. Test green deployment thoroughly")
	fmt.Println("  2. Update load balancer to point to green")
	fmt.Println("  3. Monitor for issues")
	fmt.Println("  4. Delete blue when confident:")
	fmt.Printf("     mgn workspace delete %s %s\n", *clusterID, *workspaceName)
	fmt.Println()
	fmt.Println("To rollback:")
	fmt.Println("  1. Switch load balancer back to blue")
	fmt.Println("  2. Delete green:")
	fmt.Printf("     mgn workspace delete %s %s\n", *clusterID, greenName)
	fmt.Println()
	fmt.Println("✓ Blue-Green deployment complete")
	fmt.Println("  Both versions are running, manual switch required")
}
