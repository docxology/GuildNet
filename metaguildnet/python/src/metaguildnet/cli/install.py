"""Installation automation commands"""

import subprocess

import click
from rich.console import Console

console = Console()


@click.command()
@click.option("--type", "install_type", type=click.Choice(["local", "bare-metal"]), default="local")
@click.option("--cluster-name", default="guildnet-cluster", help="Cluster name")
@click.option("--skip-verify", is_flag=True, help="Skip verification steps")
@click.option("--dry-run", is_flag=True, help="Test script paths without executing")
@click.pass_context
def install(ctx, install_type, cluster_name, skip_verify, dry_run):
    """Run automated GuildNet installation"""
    
    if dry_run:
        console.print("[bold cyan]Dry run mode - verifying script paths...[/bold cyan]\n")
        verify_scripts()
        return
    
    console.print(f"[bold]Starting GuildNet installation (type: {install_type})[/bold]\n")

    if install_type == "local":
        install_local(cluster_name, skip_verify)
    elif install_type == "bare-metal":
        install_bare_metal(cluster_name, skip_verify)


def install_local(cluster_name: str, skip_verify: bool):
    """Install GuildNet locally (microk8s)"""
    import platform
    
    # Check if running on macOS
    if platform.system() == "Darwin":
        console.print("[yellow]⚠ macOS Detected[/yellow]\n")
        console.print("The default installation uses MicroK8s (Linux only).")
        console.print("For macOS, please use one of these alternatives:\n")
        console.print("[cyan]Option 1:[/cyan] Docker Desktop with Kubernetes (Recommended)")
        console.print("  1. Enable Kubernetes in Docker Desktop Settings")
        console.print("  2. Run: cd /Users/4d/Documents/GitHub/GuildNet")
        console.print("  3. Run: kubectl create namespace guildnet")
        console.print("  4. Run: kubectl apply -f k8s/rethinkdb.yaml")
        console.print("  5. Run: ./scripts/run-hostapp.sh\n")
        console.print("[cyan]Option 2:[/cyan] Minikube")
        console.print("  brew install minikube && minikube start\n")
        console.print("[cyan]Option 3:[/cyan] Kind")
        console.print("  brew install kind && kind create cluster\n")
        console.print(f"[cyan]📖 Full guide:[/cyan] metaguildnet/docs/macos-setup.md\n")
        console.print("[yellow]To proceed with Linux-style installation anyway, use:[/yellow]")
        console.print("  mgn install --type bare-metal\n")
        return
    
    console.print("[cyan]Step 1:[/cyan] Checking prerequisites...")
    run_script("scripts/install/00-check-prereqs.sh")

    console.print("\n[cyan]Step 2:[/cyan] Installing microk8s...")
    run_script("scripts/install/01-install-microk8s.sh")

    console.print("\n[cyan]Step 3:[/cyan] Setting up Headscale...")
    run_script("scripts/install/02-setup-headscale.sh")

    console.print("\n[cyan]Step 4:[/cyan] Deploying GuildNet...")
    run_script("scripts/install/03-deploy-guildnet.sh")

    console.print("\n[cyan]Step 5:[/cyan] Bootstrapping cluster...")
    run_script("scripts/install/04-bootstrap-cluster.sh", env={"CLUSTER": cluster_name})

    if not skip_verify:
        console.print("\n[cyan]Step 6:[/cyan] Verifying installation...")
        run_script("scripts/verify/verify-all.sh")

    console.print("\n[green]✓ Installation complete![/green]")
    console.print(f"\nCluster '{cluster_name}' is ready. Access GuildNet at https://localhost:8090")


def install_bare_metal(cluster_name: str, skip_verify: bool):
    """Install GuildNet on bare metal"""
    console.print("[yellow]Bare metal installation requires manual setup.[/yellow]")
    console.print("Please refer to the installation documentation.")


def verify_scripts():
    """Verify all installation scripts exist"""
    from pathlib import Path
    from rich.table import Table
    
    base_dir = Path(__file__).parent.parent.parent.parent.parent
    
    scripts = [
        ("Step 1", "scripts/install/00-check-prereqs.sh", "Check prerequisites"),
        ("Step 2", "scripts/install/01-install-microk8s.sh", "Install MicroK8s"),
        ("Step 3", "scripts/install/02-setup-headscale.sh", "Setup Headscale"),
        ("Step 4", "scripts/install/03-deploy-guildnet.sh", "Deploy GuildNet"),
        ("Step 5", "scripts/install/04-bootstrap-cluster.sh", "Bootstrap cluster"),
        ("Verify", "scripts/verify/verify-all.sh", "Verify installation"),
    ]
    
    table = Table(title="Installation Script Verification", show_header=True)
    table.add_column("Step", style="cyan")
    table.add_column("Script", style="blue")
    table.add_column("Description")
    table.add_column("Status")
    
    all_found = True
    for step, script_path, description in scripts:
        full_path = base_dir / script_path
        if full_path.exists():
            table.add_row(step, script_path, description, "[green]✓ Found[/green]")
        else:
            table.add_row(step, script_path, description, "[red]✗ Missing[/red]")
            all_found = False
    
    console.print(table)
    console.print(f"\n[bold]Base directory:[/bold] {base_dir}")
    
    if all_found:
        console.print("\n[green]✅ All scripts found! Installation system is ready.[/green]")
        console.print("\n[yellow]Note:[/yellow] To actually install GuildNet:")
        console.print("  1. Ensure prerequisites are installed (kubectl, docker, snap on Linux)")
        console.print("  2. Run: mgn install --type local")
    else:
        console.print("\n[red]✗ Some scripts are missing![/red]")


def run_script(script_path: str, env: dict = None):
    """Run a shell script"""
    import os
    from pathlib import Path

    # Resolve script path relative to metaguildnet root
    # __file__ is at: metaguildnet/python/src/metaguildnet/cli/install.py
    # We need to go up 5 levels to get to metaguildnet/
    base_dir = Path(__file__).parent.parent.parent.parent.parent
    full_path = base_dir / script_path

    if not full_path.exists():
        console.print(f"[red]✗ Script not found: {full_path}[/red]")
        console.print(f"[yellow]Expected path: {script_path}[/yellow]")
        console.print(f"[yellow]Base directory: {base_dir}[/yellow]")
        console.print(f"[yellow]Full resolved path: {full_path}[/yellow]")
        raise click.Abort()

    try:
        script_env = os.environ.copy()
        if env:
            script_env.update(env)

        result = subprocess.run(
            ["bash", str(full_path)],
            env=script_env,
            capture_output=True,
            text=True,
        )

        if result.stdout:
            console.print(result.stdout)

        if result.returncode != 0:
            console.print(f"[red]✗ Script failed: {script_path}[/red]")
            if result.stderr:
                console.print(f"[dim]{result.stderr}[/dim]")
            raise click.Abort()

    except subprocess.CalledProcessError as e:
        console.print(f"[red]✗ Failed to run script: {e}[/red]")
        raise click.Abort()
    except Exception as e:
        console.print(f"[red]✗ Unexpected error: {e}[/red]")
        raise click.Abort()

