package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/docxology/GuildNet/internal/model"
	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
)

// This example demonstrates database operations:
// 1. Create a database
// 2. Create tables with schema
// 3. Insert data
// 4. Query data
// 5. Optionally sync data between clusters

func main() {
	apiURL := os.Getenv("MGN_API_URL")
	if apiURL == "" {
		apiURL = "https://localhost:8090"
	}

	token := os.Getenv("MGN_API_TOKEN")

	c := client.NewClient(apiURL, token)
	ctx := context.Background()

	// Get first cluster
	clusters, err := c.Clusters().List(ctx)
	if err != nil {
		log.Fatalf("Failed to list clusters: %v", err)
	}

	if len(clusters) == 0 {
		log.Fatal("No clusters available")
	}

	clusterID := clusters[0].ID
	fmt.Printf("Using cluster: %s\n", clusters[0].Name)

	// Create database
	dbName := fmt.Sprintf("example_db_%d", time.Now().Unix())
	fmt.Printf("\nCreating database: %s\n", dbName)

	db, err := c.Databases(clusterID).Create(ctx, dbName, "Example database for sync demo")
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}

	fmt.Printf("Database created: %s\n", db.ID)

	// Defer cleanup
	defer func() {
		fmt.Println("\nCleaning up...")
		err := c.Databases(clusterID).Delete(ctx, db.ID)
		if err != nil {
			log.Printf("Failed to delete database: %v", err)
		} else {
			fmt.Println("Database deleted")
		}
	}()

	// Create table
	fmt.Println("\nCreating table 'users'...")

	table := model.Table{
		Name:       "users",
		PrimaryKey: "id",
		Schema: []model.ColumnDef{
			{Name: "name", Type: "string", Required: true},
			{Name: "email", Type: "string", Required: true, Unique: true},
			{Name: "age", Type: "number", Required: false},
			{Name: "created_at", Type: "timestamp", Required: true},
		},
	}

	err = c.Databases(clusterID).CreateTable(ctx, db.ID, table)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	fmt.Println("Table created successfully")

	// Insert data
	fmt.Println("\nInserting sample data...")

	rows := []map[string]any{
		{
			"name":       "Alice Smith",
			"email":      "alice@example.com",
			"age":        30,
			"created_at": time.Now().Format(time.RFC3339),
		},
		{
			"name":       "Bob Johnson",
			"email":      "bob@example.com",
			"age":        25,
			"created_at": time.Now().Format(time.RFC3339),
		},
		{
			"name":       "Charlie Brown",
			"email":      "charlie@example.com",
			"age":        35,
			"created_at": time.Now().Format(time.RFC3339),
		},
	}

	ids, err := c.Databases(clusterID).InsertRows(ctx, db.ID, "users", rows)
	if err != nil {
		log.Fatalf("Failed to insert rows: %v", err)
	}

	fmt.Printf("Inserted %d rows (IDs: %v)\n", len(ids), ids)

	// Query data
	fmt.Println("\nQuerying data...")

	results, nextCursor, err := c.Databases(clusterID).Query(ctx, db.ID, "users", "name", 10, "", true)
	if err != nil {
		log.Fatalf("Failed to query rows: %v", err)
	}

	fmt.Printf("Query returned %d rows:\n", len(results))
	for _, row := range results {
		fmt.Printf("  - %s <%s> (age: %.0f)\n", row["name"], row["email"], row["age"])
	}

	if nextCursor != "" {
		fmt.Printf("Next cursor: %s\n", nextCursor)
	}

	// Update a row
	if len(ids) > 0 {
		fmt.Printf("\nUpdating row %s...\n", ids[0])

		patch := map[string]any{
			"age": 31,
		}

		err = c.Databases(clusterID).UpdateRow(ctx, db.ID, "users", ids[0], patch)
		if err != nil {
			log.Fatalf("Failed to update row: %v", err)
		}

		fmt.Println("Row updated successfully")

		// Get updated row
		row, err := c.Databases(clusterID).GetRow(ctx, db.ID, "users", ids[0])
		if err != nil {
			log.Fatalf("Failed to get row: %v", err)
		}

		fmt.Printf("Updated row: %s (age: %.0f)\n", row["name"], row["age"])
	}

	// List audit events
	fmt.Println("\nListing audit events...")

	events, err := c.Databases(clusterID).ListAudit(ctx, db.ID, 10)
	if err != nil {
		log.Printf("Warning: failed to list audit events: %v", err)
	} else {
		fmt.Printf("Found %d audit events:\n", len(events))
		for _, event := range events {
			fmt.Printf("  - [%s] %s by %s\n", event.TS, event.Action, event.Actor)
		}
	}

	// If multiple clusters available, demonstrate sync
	if len(clusters) > 1 {
		fmt.Println("\nMultiple clusters detected, demonstrating data sync...")

		targetCluster := clusters[1]
		fmt.Printf("Syncing to cluster: %s\n", targetCluster.Name)

		// Create database in target cluster
		targetDB, err := c.Databases(targetCluster.ID).Create(ctx, dbName, "Synced database")
		if err != nil {
			log.Printf("Failed to create target database: %v", err)
		} else {
			defer c.Databases(targetCluster.ID).Delete(ctx, targetDB.ID)

			// Create table in target
			err = c.Databases(targetCluster.ID).CreateTable(ctx, targetDB.ID, table)
			if err != nil {
				log.Printf("Failed to create target table: %v", err)
			} else {
				// Insert same data
				_, err = c.Databases(targetCluster.ID).InsertRows(ctx, targetDB.ID, "users", rows)
				if err != nil {
					log.Printf("Failed to sync data: %v", err)
				} else {
					fmt.Println("Data synced successfully!")

					// Verify sync
					targetResults, _, err := c.Databases(targetCluster.ID).Query(ctx, targetDB.ID, "users", "", 10, "", true)
					if err != nil {
						log.Printf("Failed to verify sync: %v", err)
					} else {
						fmt.Printf("Target cluster now has %d rows\n", len(targetResults))
					}
				}
			}
		}
	}

	fmt.Println("\nDatabase sync example complete!")
}
