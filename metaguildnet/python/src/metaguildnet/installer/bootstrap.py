"""Automated installation orchestrator for GuildNet."""

import os
import subprocess
import sys
from pathlib import Path
from typing import Optional

from rich.console import Console
from rich.progress import Progress, SpinnerColumn, TextColumn

console = Console()


def run_command(cmd: list[str], cwd: Optional[Path] = None, check: bool = True) -> subprocess.CompletedProcess:
    """Run a command and return the result."""
    console.print(f"[dim]Running: {' '.join(cmd)}[/dim]")
    result = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, check=False)
    
    if check and result.returncode != 0:
        console.print(f"[red]Command failed with code {result.returncode}[/red]")
        console.print(f"[red]stderr: {result.stderr}[/red]")
        raise RuntimeError(f"Command failed: {' '.join(cmd)}")
    
    return result


def check_prerequisite(name: str, command: list[str]) -> bool:
    """Check if a prerequisite is installed."""
    try:
        result = subprocess.run(command, capture_output=True, check=False)
        return result.returncode == 0
    except FileNotFoundError:
        return False


def install_dependencies() -> bool:
    """Check and install prerequisites."""
    console.print("\n[bold cyan]Checking Prerequisites[/bold cyan]")
    
    prerequisites = {
        "Docker": ["docker", "--version"],
        "kubectl": ["kubectl", "version", "--client"],
        "MicroK8s": ["microk8s", "version"],
    }
    
    missing = []
    
    with Progress(
        SpinnerColumn(),
        TextColumn("[progress.description]{task.description}"),
        console=console,
    ) as progress:
        for name, cmd in prerequisites.items():
            task = progress.add_task(f"Checking {name}...", total=1)
            
            if check_prerequisite(name, cmd):
                console.print(f"  ✓ {name} installed")
            else:
                console.print(f"  ✗ {name} not found")
                missing.append(name)
            
            progress.update(task, advance=1)
    
    if missing:
        console.print(f"\n[yellow]Missing prerequisites: {', '.join(missing)}[/yellow]")
        console.print("\nTo install missing components:")
        
        if "MicroK8s" in missing:
            console.print("  MicroK8s: sudo snap install microk8s --classic")
        if "Docker" in missing:
            console.print("  Docker: curl -fsSL https://get.docker.com | sh")
        if "kubectl" in missing:
            console.print("  kubectl: snap install kubectl --classic")
        
        return False
    
    console.print("\n[green]✓ All prerequisites installed[/green]")
    return True


def setup_headscale() -> bool:
    """Bootstrap Headscale."""
    console.print("\n[bold cyan]Setting up Headscale[/bold cyan]")
    
    # Find GuildNet scripts directory
    script_dir = Path(__file__).parent.parent.parent.parent.parent.parent / "scripts"
    
    if not script_dir.exists():
        console.print("[red]GuildNet scripts directory not found[/red]")
        return False
    
    # Run headscale bootstrap script
    headscale_script = script_dir / "headscale-bootstrap.sh"
    
    if not headscale_script.exists():
        console.print(f"[yellow]Headscale bootstrap script not found at {headscale_script}[/yellow]")
        return False
    
    try:
        console.print("Running headscale bootstrap...")
        run_command(["bash", str(headscale_script)])
        console.print("[green]✓ Headscale setup complete[/green]")
        return True
    except RuntimeError as e:
        console.print(f"[red]Headscale setup failed: {e}[/red]")
        return False


def deploy_cluster() -> bool:
    """Deploy GuildNet to cluster."""
    console.print("\n[bold cyan]Deploying GuildNet[/bold cyan]")
    
    script_dir = Path(__file__).parent.parent.parent.parent.parent.parent / "scripts"
    
    if not script_dir.exists():
        console.print("[red]GuildNet scripts directory not found[/red]")
        return False
    
    steps = [
        ("microk8s-setup.sh", "Setting up MicroK8s"),
        ("rethinkdb-setup.sh", "Setting up RethinkDB"),
        ("deploy-operator.sh", "Deploying operator"),
    ]
    
    with Progress(
        SpinnerColumn(),
        TextColumn("[progress.description]{task.description}"),
        console=console,
    ) as progress:
        for script_name, description in steps:
            task = progress.add_task(description, total=1)
            script_path = script_dir / script_name
            
            if not script_path.exists():
                console.print(f"[yellow]Script not found: {script_name}[/yellow]")
                continue
            
            try:
                run_command(["bash", str(script_path)])
                console.print(f"  ✓ {description}")
            except RuntimeError as e:
                console.print(f"[red]Failed: {description} - {e}[/red]")
                return False
            
            progress.update(task, advance=1)
    
    console.print("[green]✓ GuildNet deployment complete[/green]")
    return True


def verify_installation() -> bool:
    """Post-install verification."""
    console.print("\n[bold cyan]Verifying Installation[/bold cyan]")
    
    script_dir = Path(__file__).parent.parent.parent.parent.parent.parent / "metaguildnet" / "scripts" / "verify"
    
    if not script_dir.exists():
        console.print("[yellow]Verification scripts not found, using basic checks[/yellow]")
        script_dir = None
    
    checks = [
        ("Kubernetes cluster", ["kubectl", "cluster-info"]),
        ("GuildNet CRDs", ["kubectl", "get", "crd", "workspaces.guildnet.io"]),
        ("RethinkDB pods", ["kubectl", "get", "pods", "-l", "app=rethinkdb"]),
    ]
    
    all_passed = True
    
    with Progress(
        SpinnerColumn(),
        TextColumn("[progress.description]{task.description}"),
        console=console,
    ) as progress:
        for name, cmd in checks:
            task = progress.add_task(f"Checking {name}...", total=1)
            
            try:
                result = run_command(cmd, check=False)
                if result.returncode == 0:
                    console.print(f"  ✓ {name}")
                else:
                    console.print(f"  ✗ {name}")
                    all_passed = False
            except Exception as e:
                console.print(f"  ✗ {name}: {e}")
                all_passed = False
            
            progress.update(task, advance=1)
    
    if all_passed:
        console.print("\n[green]✓ Installation verified successfully[/green]")
        console.print("\n[bold]Next steps:[/bold]")
        console.print("  1. Start the Host App: ./scripts/run-hostapp.sh")
        console.print("  2. Bootstrap cluster: mgn cluster bootstrap")
        console.print("  3. Create a workspace: mgn workspace create <cluster-id> --name myapp --image nginx")
        return True
    else:
        console.print("\n[yellow]⚠ Some checks failed[/yellow]")
        console.print("Review the errors above and check the documentation.")
        return False


def full_install() -> bool:
    """Run full installation process."""
    console.print("[bold green]MetaGuildNet Installation[/bold green]")
    console.print("This will install and configure GuildNet on this machine.\n")
    
    steps = [
        ("Dependencies", install_dependencies),
        ("Headscale", setup_headscale),
        ("Cluster", deploy_cluster),
        ("Verification", verify_installation),
    ]
    
    for name, func in steps:
        if not func():
            console.print(f"\n[red]✗ Installation failed at: {name}[/red]")
            return False
    
    console.print("\n[bold green]✓ Installation complete![/bold green]")
    return True


if __name__ == "__main__":
    success = full_install()
    sys.exit(0 if success else 1)

