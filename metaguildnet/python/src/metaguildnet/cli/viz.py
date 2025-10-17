"""Visualization dashboard command"""

import time
from datetime import datetime

import click
from rich.console import Console
from rich.layout import Layout
from rich.live import Live
from rich.panel import Panel
from rich.table import Table

console = Console()


@click.command()
@click.option("--cluster", help="Focus on specific cluster")
@click.option("--refresh", default=5, help="Refresh interval in seconds")
@click.pass_context
def viz(ctx, cluster, refresh):
    """Launch real-time dashboard"""
    client = ctx.obj["client"]

    try:
        with Live(generate_dashboard(client, cluster), refresh_per_second=1 / refresh) as live:
            while True:
                time.sleep(refresh)
                live.update(generate_dashboard(client, cluster))

    except KeyboardInterrupt:
        console.print("\n[yellow]Dashboard closed[/yellow]")


def generate_dashboard(client, focus_cluster=None):
    """Generate dashboard layout"""
    layout = Layout()

    layout.split(
        Layout(name="header", size=3),
        Layout(name="body"),
        Layout(name="footer", size=3),
    )

    layout["body"].split_row(
        Layout(name="clusters"),
        Layout(name="workspaces"),
    )

    # Header
    layout["header"].update(
        Panel(
            f"[bold cyan]MetaGuildNet Dashboard[/bold cyan] - {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
            style="bold white on blue",
        )
    )

    # Initialize clusters variable
    clusters = []
    connection_error = False
    
    # Clusters
    try:
        clusters = client.clusters.list()
        
        if not clusters:
            layout["clusters"].update(
                Panel(
                    "[yellow]No clusters registered yet[/yellow]\n\n"
                    "To add a cluster:\n"
                    "  mgn cluster bootstrap --kubeconfig /path/to/kubeconfig\n\n"
                    "Or if GuildNet is not running:\n"
                    "  1. Start GuildNet Host App\n"
                    "  2. Bootstrap your first cluster",
                    title="Clusters"
                )
            )
        else:
            cluster_table = Table(title="Clusters", show_header=True)
            cluster_table.add_column("ID", style="cyan", no_wrap=True)
            cluster_table.add_column("Name", style="green")
            cluster_table.add_column("Status")

            for c in clusters:
                try:
                    health = client.health.cluster(c.get("id", ""))
                    if health.get("k8sReachable"):
                        status = "[green]✓ Healthy[/green]"
                    else:
                        status = f"[red]✗ Unhealthy[/red]"
                        if health.get("recommendedAction"):
                            status += f"\n[dim]{health['recommendedAction']}[/dim]"
                    cluster_table.add_row(
                        c.get("id", "")[:12] + "...",
                        c.get("name", c.get("id", "")),
                        status
                    )
                except Exception as health_err:
                    cluster_table.add_row(
                        c.get("id", "")[:12] + "...",
                        c.get("name", c.get("id", "")),
                        "[yellow]? Unknown[/yellow]"
                    )

            layout["clusters"].update(Panel(cluster_table, title=f"Clusters ({len(clusters)} total)"))
            
    except ConnectionError as e:
        connection_error = True
        layout["clusters"].update(
            Panel(
                "[red]GuildNet Host App not running[/red]\n\n"
                "[yellow]To start GuildNet:[/yellow]\n"
                "  cd /path/to/GuildNet\n"
                "  ./scripts/run-hostapp.sh\n\n"
                "Or use automated installation:\n"
                "  mgn install --type local\n\n"
                f"[dim]Error: {e}[/dim]",
                title="Connection Error",
                border_style="red"
            )
        )
    except Exception as e:
        connection_error = True
        error_msg = str(e)
        if "Connection refused" in error_msg or "Errno 61" in error_msg:
            layout["clusters"].update(
                Panel(
                    "[red]Cannot connect to GuildNet API[/red]\n\n"
                    "[yellow]Expected API URL:[/yellow] https://localhost:8090\n\n"
                    "[yellow]To fix:[/yellow]\n"
                    "  1. Ensure GuildNet Host App is running\n"
                    "  2. Check MGN_API_URL environment variable\n"
                    "  3. Verify network connectivity\n\n"
                    "Start GuildNet with:\n"
                    "  ./scripts/run-hostapp.sh\n\n"
                    f"[dim]Error: {error_msg}[/dim]",
                    title="Connection Error",
                    border_style="red"
                )
            )
        else:
            layout["clusters"].update(
                Panel(
                    f"[red]Error loading clusters:[/red]\n{error_msg}\n\n"
                    "[yellow]Check that:[/yellow]\n"
                    "  • GuildNet Host App is running\n"
                    "  • API URL is correct (MGN_API_URL)\n"
                    "  • Authentication token is valid (if required)",
                    title="Error",
                    border_style="red"
                )
            )

    # Workspaces
    if connection_error:
        layout["workspaces"].update(
            Panel(
                "[yellow]Cannot load workspaces[/yellow]\n\n"
                "GuildNet API connection required\n\n"
                "[dim]Fix the connection error first[/dim]",
                title="Workspaces",
                border_style="yellow"
            )
        )
    else:
        try:
            if focus_cluster:
                cluster_id = focus_cluster
            elif clusters and len(clusters) > 0:
                cluster_id = clusters[0].get("id", "")
            else:
                cluster_id = None

            if cluster_id:
                workspaces = client.workspaces(cluster_id).list()
                
                if not workspaces:
                    layout["workspaces"].update(
                        Panel(
                            f"[yellow]No workspaces in cluster[/yellow]\n\n"
                            f"Cluster: {cluster_id[:12]}...\n\n"
                            "To create a workspace:\n"
                            f"  mgn workspace create {cluster_id} \\\n"
                            "    --name my-workspace \\\n"
                            "    --image nginx:alpine",
                            title="Workspaces"
                        )
                    )
                else:
                    ws_table = Table(title=f"Workspaces", show_header=True)
                    ws_table.add_column("Name", style="green")
                    ws_table.add_column("Image", style="cyan")
                    ws_table.add_column("Status")
                    ws_table.add_column("Ready")

                    for ws in workspaces:
                        status_str = ws.get("status", "Unknown")
                        if status_str == "Running":
                            status_display = "[green]Running[/green]"
                        elif status_str == "Pending":
                            status_display = "[yellow]Pending[/yellow]"
                        elif status_str == "Failed":
                            status_display = "[red]Failed[/red]"
                        else:
                            status_display = status_str
                            
                        ready = ws.get("readyReplicas", 0)
                        ws_table.add_row(
                            ws.get("name", ""),
                            ws.get("image", "")[:40],
                            status_display,
                            str(ready)
                        )

                    layout["workspaces"].update(
                        Panel(
                            ws_table, 
                            title=f"Workspaces ({len(workspaces)} total) - Cluster: {cluster_id[:12]}..."
                        )
                    )
            else:
                layout["workspaces"].update(
                    Panel(
                        "[yellow]No clusters available[/yellow]\n\n"
                        "Bootstrap a cluster first:\n"
                        "  mgn cluster bootstrap --kubeconfig /path/to/config",
                        title="Workspaces"
                    )
                )

        except Exception as e:
            error_msg = str(e)
            layout["workspaces"].update(
                Panel(
                    f"[red]Error loading workspaces:[/red]\n{error_msg}",
                    title="Error",
                    border_style="red"
                )
            )

    # Footer - show helpful info
    api_url = client.base_url
    footer_text = f"[dim]API: {api_url} | Press Ctrl+C to exit | Refreshing every {5}s[/dim]"
    
    if connection_error:
        footer_text = "[yellow]⚠ Not connected to GuildNet API[/yellow] | " + footer_text
    elif clusters:
        footer_text = f"[green]✓ Connected[/green] | {len(clusters)} cluster(s) | " + footer_text
    
    layout["footer"].update(Panel(footer_text, style="dim"))

    return layout

