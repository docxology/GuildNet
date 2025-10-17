package tests

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMetaGuildNetStructure verifies the MetaGuildNet directory structure is complete
func TestMetaGuildNetStructure(t *testing.T) {
	// Get metaguildnet directory (this test is in metaguildnet/tests/)
	root := ".."

	// Required directories
	requiredDirs := []string{
		"docs",
		"sdk/go/client",
		"sdk/go/testing",
		"sdk/go/examples/basic-workflow",
		"sdk/go/examples/multi-cluster",
		"sdk/go/examples/database-sync",
		"python/src/metaguildnet/cli",
		"python/src/metaguildnet/api",
		"python/src/metaguildnet/config",
		"python/src/metaguildnet/installer",
		"python/src/metaguildnet/visualizer",
		"orchestrator/templates",
		"orchestrator/examples/multi-cluster",
		"orchestrator/examples/lifecycle",
		"orchestrator/examples/cicd",
		"scripts/install",
		"scripts/verify",
		"scripts/utils",
		"tests/integration",
		"tests/e2e",
	}

	for _, dir := range requiredDirs {
		path := filepath.Join(root, dir)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Required directory missing: %s", dir)
		}
	}

	// Required files
	requiredFiles := []string{
		"README.md",
		"QUICKSTART.md",
		"IMPLEMENTATION_SUMMARY.md",
		"docs/getting-started.md",
		"docs/concepts.md",
		"docs/examples.md",
		"docs/api-reference.md",
		"sdk/go/client/guildnet.go",
		"sdk/go/client/cluster.go",
		"sdk/go/client/workspace.go",
		"sdk/go/client/database.go",
		"sdk/go/client/health.go",
		"python/pyproject.toml",
		"python/src/metaguildnet/__init__.py",
		"python/src/metaguildnet/cli/main.py",
		"python/src/metaguildnet/api/client.py",
		"scripts/install/install-all.sh",
		"scripts/verify/verify-all.sh",
		"scripts/utils/log-collector.sh",
		"scripts/utils/debug-info.sh",
		"scripts/utils/cleanup.sh",
		"scripts/utils/backup-config.sh",
	}

	for _, file := range requiredFiles {
		path := filepath.Join(root, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Required file missing: %s", file)
		}
	}

	t.Log("✓ MetaGuildNet structure validation complete")
}

// TestShellScriptsExecutable verifies all shell scripts are executable
func TestShellScriptsExecutable(t *testing.T) {
	root := ".."

	scriptDirs := []string{
		"scripts/install",
		"scripts/verify",
		"scripts/utils",
		"orchestrator/examples/lifecycle",
		"orchestrator/examples/multi-cluster",
	}

	for _, dir := range scriptDirs {
		dirPath := filepath.Join(root, dir)
		files, err := os.ReadDir(dirPath)
		if err != nil {
			t.Errorf("Failed to read directory %s: %v", dir, err)
			continue
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			if filepath.Ext(file.Name()) == ".sh" {
				filePath := filepath.Join(dirPath, file.Name())
				info, err := os.Stat(filePath)
				if err != nil {
					t.Errorf("Failed to stat %s: %v", filePath, err)
					continue
				}

				mode := info.Mode()
				if mode&0111 == 0 {
					t.Errorf("Script not executable: %s", filePath)
				}
			}
		}
	}

	t.Log("✓ Shell script permissions validated")
}
