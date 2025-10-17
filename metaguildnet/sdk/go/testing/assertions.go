package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/docxology/GuildNet/metaguildnet/sdk/go/client"
)

// AssertClusterHealthy asserts that a cluster is healthy
func AssertClusterHealthy(t *testing.T, c *client.Client, clusterID string) {
	t.Helper()

	ctx := context.Background()
	health, err := c.Health().Cluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("failed to get cluster health: %v", err)
	}

	if !health.K8sReachable {
		t.Errorf("cluster %s is not healthy: k8s unreachable", clusterID)
	}

	if !health.KubeconfigValid {
		t.Errorf("cluster %s has invalid kubeconfig", clusterID)
	}

	if health.K8sError != "" {
		t.Errorf("cluster %s has k8s error: %s", clusterID, health.K8sError)
	}
}

// AssertWorkspaceRunning asserts that a workspace is in Running state
func AssertWorkspaceRunning(t *testing.T, c *client.Client, clusterID, name string) {
	t.Helper()

	ctx := context.Background()
	ws, err := c.Workspaces(clusterID).Get(ctx, name)
	if err != nil {
		t.Fatalf("failed to get workspace %s: %v", name, err)
	}

	if ws.Status != "Running" {
		t.Errorf("workspace %s is not running (status: %s)", name, ws.Status)
	}

	if ws.ReadyReplicas < 1 {
		t.Errorf("workspace %s has no ready replicas", name)
	}
}

// AssertWorkspaceExists asserts that a workspace exists
func AssertWorkspaceExists(t *testing.T, c *client.Client, clusterID, name string) {
	t.Helper()

	ctx := context.Background()
	_, err := c.Workspaces(clusterID).Get(ctx, name)
	if err != nil {
		t.Fatalf("workspace %s does not exist: %v", name, err)
	}
}

// AssertWorkspaceNotExists asserts that a workspace does not exist
func AssertWorkspaceNotExists(t *testing.T, c *client.Client, clusterID, name string) {
	t.Helper()

	ctx := context.Background()
	_, err := c.Workspaces(clusterID).Get(ctx, name)
	if err == nil {
		t.Fatalf("workspace %s still exists", name)
	}
}

// AssertDatabaseExists asserts that a database exists
func AssertDatabaseExists(t *testing.T, c *client.Client, clusterID, dbID string) {
	t.Helper()

	ctx := context.Background()
	_, err := c.Databases(clusterID).Get(ctx, dbID)
	if err != nil {
		t.Fatalf("database %s does not exist: %v", dbID, err)
	}
}

// AssertTableExists asserts that a table exists in a database
func AssertTableExists(t *testing.T, c *client.Client, clusterID, dbID, table string) {
	t.Helper()

	ctx := context.Background()
	tables, err := c.Databases(clusterID).Tables(ctx, dbID)
	if err != nil {
		t.Fatalf("failed to list tables: %v", err)
	}

	found := false
	for _, tbl := range tables {
		if tbl.Name == table {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("table %s does not exist in database %s", table, dbID)
	}
}

// AssertRowCount asserts that a table has the expected number of rows
func AssertRowCount(t *testing.T, c *client.Client, clusterID, dbID, table string, expected int) {
	t.Helper()

	ctx := context.Background()
	rows, _, err := c.Databases(clusterID).Query(ctx, dbID, table, "", 1000, "", true)
	if err != nil {
		t.Fatalf("failed to query rows: %v", err)
	}

	if len(rows) != expected {
		t.Errorf("expected %d rows in table %s, got %d", expected, table, len(rows))
	}
}

// WaitForCondition waits for a condition to be true
func WaitForCondition(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if condition() {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for condition: %s", message)
		}

		<-ticker.C
	}
}

// AssertEventually asserts that a condition eventually becomes true
func AssertEventually(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	t.Helper()
	WaitForCondition(t, timeout, condition, message)
}

// AssertNoError asserts that an error is nil
func AssertNoError(t *testing.T, err error, message string) {
	t.Helper()

	if err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

// AssertError asserts that an error occurred
func AssertError(t *testing.T, err error, message string) {
	t.Helper()

	if err == nil {
		t.Fatalf("%s: expected error but got nil", message)
	}
}

// AssertEqual asserts that two values are equal
func AssertEqual(t *testing.T, expected, actual interface{}, message string) {
	t.Helper()

	if expected != actual {
		t.Errorf("%s: expected %v, got %v", message, expected, actual)
	}
}

// AssertContains asserts that a slice contains an element
func AssertContains(t *testing.T, slice []string, element string, message string) {
	t.Helper()

	for _, item := range slice {
		if item == element {
			return
		}
	}

	t.Errorf("%s: slice does not contain %s", message, element)
}

// AssertGreaterThan asserts that a value is greater than another
func AssertGreaterThan(t *testing.T, value, threshold int, message string) {
	t.Helper()

	if value <= threshold {
		t.Errorf("%s: expected %d > %d", message, value, threshold)
	}
}

// WithTimeout runs a test function with a timeout
func WithTimeout(t *testing.T, timeout time.Duration, fn func()) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
		// Success
	case <-time.After(timeout):
		t.Fatal("test timed out")
	}
}

// Retry retries a function until it succeeds or max attempts reached
func Retry(t *testing.T, maxAttempts int, delay time.Duration, fn func() error) error {
	t.Helper()

	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if i > 0 {
			time.Sleep(delay)
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}
