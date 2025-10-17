"""Workspace management commands"""

import click
from rich.console import Console
from rich.table import Table

console = Console()


@click.group()
def workspace():
    """Manage workspaces"""
    pass


@workspace.command("list")
@click.argument("cluster_id")
@click.pass_context
def list_workspaces(ctx, cluster_id):
    """List workspaces in a cluster"""
    client = ctx.obj["client"]

    try:
        workspaces = client.workspaces(cluster_id).list()

        table = Table(title=f"Workspaces in {cluster_id}")
        table.add_column("Name", style="cyan")
        table.add_column("Image", style="green")
        table.add_column("Status")
        table.add_column("Ports")

        for ws in workspaces:
            ports = ", ".join(str(p.get("port", "")) for p in ws.get("ports", []))
            table.add_row(
                ws.get("name", ""), ws.get("image", ""), ws.get("status", ""), ports
            )

        console.print(table)

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@workspace.command("create")
@click.argument("cluster_id")
@click.option("--name", required=True, help="Workspace name")
@click.option("--image", required=True, help="Container image")
@click.option("--env", multiple=True, help="Environment variable (KEY=VALUE)")
@click.option("--port", multiple=True, type=int, help="Container port")
@click.pass_context
def create_workspace(ctx, cluster_id, name, image, env, port):
    """Create a new workspace"""
    client = ctx.obj["client"]

    try:
        spec = {"name": name, "image": image}

        if env:
            spec["env"] = [{"name": k, "value": v} for e in env for k, v in [e.split("=", 1)]]

        if port:
            spec["ports"] = [{"containerPort": p} for p in port]

        ws = client.workspaces(cluster_id).create(spec)
        console.print(f"[green]✓[/green] Workspace created: {ws.get('name')}")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@workspace.command("delete")
@click.argument("cluster_id")
@click.argument("name")
@click.pass_context
def delete_workspace(ctx, cluster_id, name):
    """Delete a workspace"""
    client = ctx.obj["client"]

    try:
        client.workspaces(cluster_id).delete(name)
        console.print(f"[green]✓[/green] Workspace deleted: {name}")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@workspace.command("logs")
@click.argument("cluster_id")
@click.argument("name")
@click.option("--tail", default=100, help="Number of lines to show")
@click.option("--follow", "-f", is_flag=True, help="Follow log output")
@click.pass_context
def workspace_logs(ctx, cluster_id, name, tail, follow):
    """Get workspace logs"""
    client = ctx.obj["client"]

    try:
        logs = client.workspaces(cluster_id).logs(name, tail_lines=tail)

        for log in logs:
            console.print(f"[dim]{log.get('timestamp', '')}[/dim] {log.get('line', '')}")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@workspace.command("wait")
@click.argument("cluster_id")
@click.argument("name")
@click.option("--timeout", default="5m", help="Timeout duration")
@click.pass_context
def wait_workspace(ctx, cluster_id, name, timeout):
    """Wait for workspace to be ready"""
    client = ctx.obj["client"]

    try:
        import time

        console.print(f"Waiting for {name} to be ready...")

        # Simple polling implementation
        for _ in range(60):  # 5 minutes
            ws = client.workspaces(cluster_id).get(name)
            status = ws.get("status", "")

            if status == "Running":
                console.print(f"[green]✓[/green] Workspace is ready")
                return

            if status == "Failed":
                console.print(f"[red]✗[/red] Workspace failed")
                raise click.Abort()

            time.sleep(5)

        console.print(f"[red]✗[/red] Timeout waiting for workspace")
        raise click.Abort()

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()

