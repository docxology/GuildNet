"""Python API client for GuildNet"""

import base64
from typing import Any, Dict, List, Optional

import httpx


class APIError(Exception):
    """Base exception for API errors"""

    pass


class NotFoundError(APIError):
    """Resource not found"""

    pass


class UnauthorizedError(APIError):
    """Unauthorized access"""

    pass


class ClusterAPI:
    """Cluster operations"""

    def __init__(self, client: "Client"):
        self.client = client

    def list(self) -> List[Dict[str, Any]]:
        """List all clusters"""
        response = self.client._get("/api/deploy/clusters")
        return response.get("clusters", [])

    def get(self, cluster_id: str) -> Dict[str, Any]:
        """Get cluster details"""
        return self.client._get(f"/api/deploy/clusters/{cluster_id}")

    def bootstrap(self, kubeconfig: bytes) -> str:
        """Bootstrap a new cluster"""
        payload = {
            "cluster": {"kubeconfig": base64.b64encode(kubeconfig).decode()}
        }
        response = self.client._post("/bootstrap", payload)
        return response.get("clusterId", "")

    def update_settings(self, cluster_id: str, settings: Dict[str, Any]) -> None:
        """Update cluster settings"""
        self.client._put(f"/api/settings/cluster/{cluster_id}", settings)

    def get_settings(self, cluster_id: str) -> Dict[str, Any]:
        """Get cluster settings"""
        return self.client._get(f"/api/settings/cluster/{cluster_id}")


class WorkspaceAPI:
    """Workspace operations for a specific cluster"""

    def __init__(self, client: "Client", cluster_id: str):
        self.client = client
        self.cluster_id = cluster_id

    def list(self) -> List[Dict[str, Any]]:
        """List workspaces"""
        response = self.client._get(f"/api/cluster/{self.cluster_id}/servers")
        return response.get("servers", [])

    def create(self, spec: Dict[str, Any]) -> Dict[str, Any]:
        """Create workspace"""
        return self.client._post(f"/api/cluster/{self.cluster_id}/workspaces", spec)

    def get(self, name: str) -> Dict[str, Any]:
        """Get workspace details"""
        return self.client._get(f"/api/cluster/{self.cluster_id}/workspaces/{name}")

    def delete(self, name: str) -> None:
        """Delete workspace"""
        self.client._delete(f"/api/cluster/{self.cluster_id}/workspaces/{name}")

    def logs(self, name: str, tail_lines: int = 100) -> List[Dict[str, Any]]:
        """Get workspace logs"""
        path = f"/api/cluster/{self.cluster_id}/workspaces/{name}/logs?tail={tail_lines}"
        return self.client._get(path)


class DatabaseAPI:
    """Database operations for a specific cluster"""

    def __init__(self, client: "Client", cluster_id: str):
        self.client = client
        self.cluster_id = cluster_id

    def list(self) -> List[Dict[str, Any]]:
        """List databases"""
        return self.client._get(f"/api/cluster/{self.cluster_id}/db")

    def create(self, name: str, description: str = "") -> Dict[str, Any]:
        """Create database"""
        payload = {"name": name, "description": description}
        return self.client._post(f"/api/cluster/{self.cluster_id}/db", payload)

    def get(self, db_id: str) -> Dict[str, Any]:
        """Get database details"""
        return self.client._get(f"/api/cluster/{self.cluster_id}/db/{db_id}")

    def delete(self, db_id: str) -> None:
        """Delete database"""
        self.client._delete(f"/api/cluster/{self.cluster_id}/db/{db_id}")

    def tables(self, db_id: str) -> List[Dict[str, Any]]:
        """List tables in database"""
        return self.client._get(f"/api/cluster/{self.cluster_id}/db/{db_id}/tables")

    def create_table(self, db_id: str, table: Dict[str, Any]) -> None:
        """Create table"""
        self.client._post(f"/api/cluster/{self.cluster_id}/db/{db_id}/tables", table)

    def query(
        self, db_id: str, table: str, limit: int = 100
    ) -> List[Dict[str, Any]]:
        """Query table rows"""
        path = f"/api/cluster/{self.cluster_id}/db/{db_id}/tables/{table}/rows?limit={limit}"
        response = self.client._get(path)
        return response.get("rows", [])

    def insert(self, db_id: str, table: str, rows: List[Dict[str, Any]]) -> List[str]:
        """Insert rows"""
        payload = {"rows": rows}
        response = self.client._post(
            f"/api/cluster/{self.cluster_id}/db/{db_id}/tables/{table}/rows", payload
        )
        return response.get("ids", [])


class HealthAPI:
    """Health and status operations"""

    def __init__(self, client: "Client"):
        self.client = client

    def global_health(self) -> Dict[str, Any]:
        """Get global health"""
        return self.client._get("/api/health")

    def cluster(self, cluster_id: str) -> Dict[str, Any]:
        """Get cluster health"""
        return self.client._get(f"/api/cluster/{cluster_id}/health")

    def status(self) -> bool:
        """Quick health check"""
        try:
            self.client._get("/healthz")
            return True
        except Exception:
            return False


class Client:
    """MetaGuildNet API client"""

    def __init__(
        self,
        base_url: str = "https://localhost:8090",
        token: str = "",
        timeout: float = 30.0,
        verify: bool = False,
    ):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout
        self.verify = verify

        self.clusters = ClusterAPI(self)
        self.health = HealthAPI(self)

    def workspaces(self, cluster_id: str) -> WorkspaceAPI:
        """Get workspace API for cluster"""
        return WorkspaceAPI(self, cluster_id)

    def databases(self, cluster_id: str) -> DatabaseAPI:
        """Get database API for cluster"""
        return DatabaseAPI(self, cluster_id)

    def _get(self, path: str) -> Any:
        """Execute GET request"""
        return self._request("GET", path)

    def _post(self, path: str, data: Any = None) -> Any:
        """Execute POST request"""
        return self._request("POST", path, json=data)

    def _put(self, path: str, data: Any = None) -> Any:
        """Execute PUT request"""
        return self._request("PUT", path, json=data)

    def _delete(self, path: str) -> None:
        """Execute DELETE request"""
        self._request("DELETE", path)

    def _request(self, method: str, path: str, **kwargs) -> Any:
        """Execute HTTP request"""
        url = self.base_url + path

        headers = kwargs.pop("headers", {})
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"

        with httpx.Client(timeout=self.timeout, verify=self.verify) as client:
            response = client.request(method, url, headers=headers, **kwargs)

            if response.status_code == 404:
                raise NotFoundError(f"Resource not found: {path}")
            elif response.status_code in (401, 403):
                raise UnauthorizedError("Unauthorized")
            elif response.status_code >= 400:
                raise APIError(
                    f"API error {response.status_code}: {response.text}"
                )

            if method == "DELETE":
                return None

            if response.text:
                return response.json()

            return None

