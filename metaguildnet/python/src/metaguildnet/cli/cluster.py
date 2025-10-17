"""Cluster management commands"""

import json

import click
from rich.console import Console
from rich.table import Table

console = Console()


@click.group()
def cluster():
    """Manage clusters"""
    pass


@cluster.command("list")
@click.option("--format", type=click.Choice(["table", "json", "yaml"]), default="table")
@click.pass_context
def list_clusters(ctx, format):
    """List all clusters"""
    client = ctx.obj["client"]

    try:
        clusters = client.clusters.list()

        if format == "json":
            console.print_json(data=clusters)
        elif format == "yaml":
            import yaml

            console.print(yaml.dump(clusters, default_flow_style=False))
        else:
            table = Table(title="Clusters")
            table.add_column("ID", style="cyan")
            table.add_column("Name", style="green")
            table.add_column("Namespace")
            table.add_column("Ingress Domain")

            for cluster in clusters:
                table.add_row(
                    cluster.get("id", ""),
                    cluster.get("name", ""),
                    cluster.get("namespace", "default"),
                    cluster.get("ingress_domain", ""),
                )

            console.print(table)

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@cluster.command("get")
@click.argument("cluster_id")
@click.option("-o", "--output", type=click.Choice(["yaml", "json"]), default="yaml")
@click.pass_context
def get_cluster(ctx, cluster_id, output):
    """Get cluster details"""
    client = ctx.obj["client"]

    try:
        cluster = client.clusters.get(cluster_id)

        if output == "json":
            console.print_json(data=cluster)
        else:
            import yaml

            console.print(yaml.dump(cluster, default_flow_style=False))

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@cluster.command("status")
@click.argument("cluster_id")
@click.pass_context
def cluster_status(ctx, cluster_id):
    """Check cluster health status"""
    client = ctx.obj["client"]

    try:
        health = client.health.cluster(cluster_id)

        table = Table(title=f"Cluster Health: {cluster_id}")
        table.add_column("Check", style="cyan")
        table.add_column("Status", style="green")

        def status_icon(value):
            return "✓" if value else "✗"

        table.add_row("Kubeconfig Present", status_icon(health.get("kubeconfigPresent", False)))
        table.add_row("Kubeconfig Valid", status_icon(health.get("kubeconfigValid", False)))
        table.add_row("K8s Reachable", status_icon(health.get("k8sReachable", False)))

        if health.get("k8sError"):
            table.add_row("K8s Error", health["k8sError"])

        if health.get("recommendedAction"):
            table.add_row("Recommended Action", health["recommendedAction"])

        console.print(table)

        # Exit with error if unhealthy
        if not health.get("k8sReachable"):
            sys.exit(1)

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@cluster.command("bootstrap")
@click.option("--kubeconfig", type=click.Path(exists=True), required=True)
@click.option("--name", help="Cluster name")
@click.pass_context
def bootstrap_cluster(ctx, kubeconfig, name):
    """Bootstrap a new cluster"""
    client = ctx.obj["client"]

    try:
        with open(kubeconfig, "rb") as f:
            kubeconfig_data = f.read()

        cluster_id = client.clusters.bootstrap(kubeconfig_data)

        console.print(f"[green]✓[/green] Cluster bootstrapped: {cluster_id}")

        if name:
            # Update cluster name
            client.clusters.update_settings(cluster_id, {"name": name})
            console.print(f"[green]✓[/green] Cluster name set to: {name}")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@cluster.command("update")
@click.argument("cluster_id")
@click.option("--setting", multiple=True, help="Setting in key=value format")
@click.pass_context
def update_cluster(ctx, cluster_id, setting):
    """Update cluster settings"""
    client = ctx.obj["client"]

    try:
        settings = {}
        for s in setting:
            key, value = s.split("=", 1)
            # Try to parse as bool or keep as string
            if value.lower() in ("true", "false"):
                value = value.lower() == "true"
            settings[key] = value

        client.clusters.update_settings(cluster_id, settings)
        console.print(f"[green]✓[/green] Cluster settings updated")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()

