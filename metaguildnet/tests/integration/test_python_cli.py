"""Integration tests for Python CLI"""

import json
import subprocess
import time

import pytest
from metaguildnet.api.client import Client


@pytest.fixture
def client():
    """Create API client"""
    return Client("https://localhost:8090", "")


@pytest.fixture
def test_cluster(client):
    """Get first available cluster for testing"""
    clusters = client.clusters.list()
    if not clusters:
        pytest.skip("No clusters available")
    return clusters[0]["id"]


def run_mgn(args):
    """Run mgn command"""
    result = subprocess.run(
        ["mgn"] + args,
        capture_output=True,
        text=True,
    )
    return result


class TestCLICommands:
    """Test CLI commands"""

    def test_version(self):
        """Test version command"""
        result = run_mgn(["version"])
        assert result.returncode == 0
        assert "MetaGuildNet" in result.stdout

    def test_cluster_list(self):
        """Test cluster list command"""
        result = run_mgn(["cluster", "list", "--format", "json"])
        assert result.returncode == 0
        data = json.loads(result.stdout)
        assert isinstance(data, list)

    def test_verify_system(self):
        """Test system verification"""
        result = run_mgn(["verify", "system"])
        # May fail if tools missing, but should run
        assert "kubectl" in result.stdout or "docker" in result.stdout


class TestAPIClient:
    """Test Python API client"""

    def test_list_clusters(self, client):
        """Test listing clusters"""
        clusters = client.clusters.list()
        assert isinstance(clusters, list)

    def test_cluster_health(self, client, test_cluster):
        """Test cluster health check"""
        health = client.health.cluster(test_cluster)
        assert "k8sReachable" in health
        assert isinstance(health["k8sReachable"], bool)

    def test_workspace_lifecycle(self, client, test_cluster):
        """Test workspace creation and deletion"""
        workspace_name = f"test-{int(time.time())}"

        # Create
        spec = {
            "name": workspace_name,
            "image": "nginx:alpine",
        }

        ws = client.workspaces(test_cluster).create(spec)
        assert ws["name"] == workspace_name

        # Wait a bit
        time.sleep(2)

        # Get
        fetched = client.workspaces(test_cluster).get(workspace_name)
        assert fetched["name"] == workspace_name

        # Delete
        client.workspaces(test_cluster).delete(workspace_name)

        # Verify deleted
        with pytest.raises(Exception):
            client.workspaces(test_cluster).get(workspace_name)


class TestEndToEnd:
    """End-to-end workflow tests"""

    def test_complete_workflow(self, client, test_cluster):
        """Test complete workflow from creation to deletion"""
        workspace_name = f"e2e-test-{int(time.time())}"

        try:
            # Create workspace
            spec = {
                "name": workspace_name,
                "image": "nginx:alpine",
                "labels": {"test": "e2e"},
            }

            ws = client.workspaces(test_cluster).create(spec)
            assert ws is not None

            # Wait for ready (with timeout)
            max_wait = 120  # 2 minutes
            waited = 0

            while waited < max_wait:
                try:
                    fetched = client.workspaces(test_cluster).get(workspace_name)
                    if fetched.get("status") == "Running":
                        break
                except Exception:
                    pass

                time.sleep(5)
                waited += 5

            # Get logs
            logs = client.workspaces(test_cluster).logs(workspace_name, tail_lines=10)
            assert isinstance(logs, list)

        finally:
            # Cleanup
            try:
                client.workspaces(test_cluster).delete(workspace_name)
            except Exception as e:
                print(f"Cleanup failed: {e}")


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

