"""Terminal dashboard for GuildNet monitoring."""

import time
from datetime import datetime
from typing import Optional

from rich.console import Console
from rich.layout import Layout
from rich.live import Live
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from ..api.client import Client


class Dashboard:
    """Real-time terminal dashboard for GuildNet monitoring."""
    
    def __init__(self, api_url: str, api_token: Optional[str] = None):
        """Initialize dashboard with API client."""
        self.client = Client(api_url, api_token)
        self.console = Console()
        self.running = False
    
    def create_cluster_table(self) -> Table:
        """Create table showing cluster status."""
        table = Table(title="Clusters", show_header=True, header_style="bold cyan")
        table.add_column("Name", style="cyan")
        table.add_column("ID", style="dim")
        table.add_column("Status", justify="center")
        table.add_column("Nodes", justify="right")
        table.add_column("Workspaces", justify="right")
        
        try:
            clusters = self.client.list_clusters()
            
            for cluster in clusters:
                status_color = "green" if cluster.get("healthy", False) else "red"
                status_text = "●" if cluster.get("healthy", False) else "○"
                
                table.add_row(
                    cluster.get("name", "Unknown"),
                    cluster.get("id", "")[:12],
                    f"[{status_color}]{status_text}[/{status_color}]",
                    str(cluster.get("node_count", "?")),
                    str(cluster.get("workspace_count", "?")),
                )
        except Exception as e:
            table.add_row("Error", str(e), "✗", "-", "-")
        
        return table
    
    def create_workspace_table(self, cluster_id: Optional[str] = None) -> Table:
        """Create table showing workspace status."""
        table = Table(title="Workspaces", show_header=True, header_style="bold magenta")
        table.add_column("Name", style="magenta")
        table.add_column("Cluster", style="dim")
        table.add_column("Image", style="blue")
        table.add_column("Status", justify="center")
        table.add_column("Created", style="dim")
        
        try:
            # If cluster_id provided, only show workspaces from that cluster
            if cluster_id:
                workspaces = self.client.list_workspaces(cluster_id)
                cluster_map = {cluster_id: cluster_id[:12]}
            else:
                # Otherwise, get workspaces from all clusters
                clusters = self.client.list_clusters()
                workspaces = []
                cluster_map = {}
                
                for cluster in clusters:
                    cid = cluster.get("id")
                    cluster_map[cid] = cluster.get("name", cid[:12])
                    try:
                        ws_list = self.client.list_workspaces(cid)
                        for ws in ws_list:
                            ws["_cluster_id"] = cid
                            workspaces.append(ws)
                    except Exception:
                        continue
            
            for ws in workspaces[:20]:  # Limit to 20 for display
                status = ws.get("status", "Unknown")
                status_color = {
                    "Running": "green",
                    "Pending": "yellow",
                    "Failed": "red",
                }.get(status, "white")
                
                cluster_name = cluster_map.get(ws.get("_cluster_id", cluster_id), "?")
                created = ws.get("created_at", "")
                if created:
                    try:
                        created_dt = datetime.fromisoformat(created.replace("Z", "+00:00"))
                        created = created_dt.strftime("%Y-%m-%d %H:%M")
                    except Exception:
                        pass
                
                table.add_row(
                    ws.get("name", "Unknown"),
                    cluster_name,
                    ws.get("image", "")[:30],
                    f"[{status_color}]{status}[/{status_color}]",
                    created,
                )
        except Exception as e:
            table.add_row("Error", str(e), "-", "✗", "-")
        
        return table
    
    def create_health_panel(self) -> Panel:
        """Create panel showing overall health."""
        try:
            health = self.client.health()
            
            status = health.get("status", "unknown")
            status_color = {
                "healthy": "green",
                "degraded": "yellow",
                "unhealthy": "red",
            }.get(status, "white")
            
            text = Text()
            text.append("Overall Status: ", style="bold")
            text.append(status.upper(), style=f"bold {status_color}")
            text.append("\n\n")
            
            # Add component statuses
            components = health.get("components", {})
            for component, comp_status in components.items():
                comp_color = "green" if comp_status == "ok" else "red"
                text.append(f"  {component}: ", style="dim")
                text.append(comp_status, style=comp_color)
                text.append("\n")
            
            return Panel(text, title="System Health", border_style=status_color)
        except Exception as e:
            return Panel(f"Error: {e}", title="System Health", border_style="red")
    
    def create_stats_panel(self) -> Panel:
        """Create panel showing statistics."""
        try:
            clusters = self.client.list_clusters()
            total_workspaces = 0
            total_nodes = 0
            
            for cluster in clusters:
                total_workspaces += cluster.get("workspace_count", 0)
                total_nodes += cluster.get("node_count", 0)
            
            text = Text()
            text.append(f"Clusters: ", style="bold")
            text.append(f"{len(clusters)}\n", style="cyan")
            text.append(f"Nodes: ", style="bold")
            text.append(f"{total_nodes}\n", style="green")
            text.append(f"Workspaces: ", style="bold")
            text.append(f"{total_workspaces}\n", style="magenta")
            text.append(f"\nUpdated: ", style="dim")
            text.append(datetime.now().strftime("%H:%M:%S"), style="dim")
            
            return Panel(text, title="Statistics", border_style="blue")
        except Exception as e:
            return Panel(f"Error: {e}", title="Statistics", border_style="red")
    
    def create_layout(self) -> Layout:
        """Create the dashboard layout."""
        layout = Layout()
        
        layout.split_column(
            Layout(name="header", size=3),
            Layout(name="body"),
            Layout(name="footer", size=3),
        )
        
        layout["body"].split_row(
            Layout(name="left"),
            Layout(name="right", ratio=2),
        )
        
        layout["left"].split_column(
            Layout(name="health"),
            Layout(name="stats"),
        )
        
        layout["right"].split_column(
            Layout(name="clusters", ratio=1),
            Layout(name="workspaces", ratio=2),
        )
        
        return layout
    
    def update_layout(self, layout: Layout) -> None:
        """Update the layout with current data."""
        # Header
        header = Text("GuildNet Dashboard", style="bold white on blue", justify="center")
        layout["header"].update(Panel(header, border_style="blue"))
        
        # Health and stats
        layout["health"].update(self.create_health_panel())
        layout["stats"].update(self.create_stats_panel())
        
        # Clusters and workspaces
        layout["clusters"].update(Panel(self.create_cluster_table(), border_style="cyan"))
        layout["workspaces"].update(Panel(self.create_workspace_table(), border_style="magenta"))
        
        # Footer
        footer = Text(
            "Press Ctrl+C to exit | Updates every 5 seconds",
            style="dim",
            justify="center"
        )
        layout["footer"].update(Panel(footer, border_style="dim"))
    
    def run(self, refresh_interval: int = 5) -> None:
        """Run the dashboard with live updates."""
        layout = self.create_layout()
        self.running = True
        
        try:
            with Live(layout, console=self.console, refresh_per_second=1, screen=True):
                while self.running:
                    self.update_layout(layout)
                    time.sleep(refresh_interval)
        except KeyboardInterrupt:
            self.console.print("\n[yellow]Dashboard stopped[/yellow]")
            self.running = False
    
    def snapshot(self) -> None:
        """Display a single snapshot of the dashboard (non-interactive)."""
        layout = self.create_layout()
        self.update_layout(layout)
        self.console.print(layout)


def main():
    """Run dashboard as standalone application."""
    import os
    import sys
    
    api_url = os.getenv("MGN_API_URL", "https://localhost:8090")
    api_token = os.getenv("MGN_API_TOKEN")
    
    if not api_url:
        print("Error: MGN_API_URL environment variable not set", file=sys.stderr)
        sys.exit(1)
    
    dashboard = Dashboard(api_url, api_token)
    
    # Check for snapshot mode
    if len(sys.argv) > 1 and sys.argv[1] == "--snapshot":
        dashboard.snapshot()
    else:
        dashboard.run()


if __name__ == "__main__":
    main()

