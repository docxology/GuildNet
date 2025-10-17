"""Verification commands"""

import subprocess
from pathlib import Path

import click
from rich.console import Console

console = Console()


@click.group()
def verify():
    """Verify system and installation"""
    pass


@verify.command()
def system():
    """Verify system prerequisites"""
    console.print("[bold]Checking system prerequisites...[/bold]\n")
    run_verify_script("verify-system.sh")


@verify.command()
def network():
    """Verify network connectivity"""
    console.print("[bold]Checking network connectivity...[/bold]\n")
    run_verify_script("verify-network.sh")


@verify.command()
def kubernetes():
    """Verify Kubernetes cluster"""
    console.print("[bold]Checking Kubernetes cluster...[/bold]\n")
    run_verify_script("verify-kubernetes.sh")


@verify.command()
def guildnet():
    """Verify GuildNet installation"""
    console.print("[bold]Checking GuildNet installation...[/bold]\n")
    run_verify_script("verify-guildnet.sh")


@verify.command()
def all():
    """Run all verification checks"""
    console.print("[bold]Running all verification checks...[/bold]\n")
    run_verify_script("verify-all.sh")


def run_verify_script(script_name: str):
    """Run a verification script"""
    base_dir = Path(__file__).parent.parent.parent.parent.parent
    script_path = base_dir / "scripts" / "verify" / script_name

    if not script_path.exists():
        console.print(f"[yellow]⚠ Script not found: {script_path}[/yellow]")
        console.print("[cyan]Performing basic checks instead...[/cyan]\n")
        perform_basic_checks(script_name)
        return

    try:
        result = subprocess.run(
            ["bash", str(script_path)],
            capture_output=True,
            text=True,
        )

        if result.stdout:
            console.print(result.stdout)

        if result.returncode != 0:
            # Don't abort - just report the issues
            if result.stderr:
                console.print(f"[dim]{result.stderr}[/dim]")
            
            # Provide helpful context
            if "verify-all" in script_name or script_name == "verify-all.sh":
                console.print("\n[yellow]⚠ Some checks failed (this is expected if GuildNet is not installed yet)[/yellow]")
                console.print("\n[cyan]To install GuildNet:[/cyan]")
                console.print("  mgn install --type local")
                console.print("\n[cyan]Or manually:[/cyan]")
                console.print("  cd /path/to/GuildNet")
                console.print("  ./scripts/run-hostapp.sh")
        else:
            console.print("\n[green]✅ All verifications passed![/green]")

    except subprocess.CalledProcessError as e:
        console.print(f"[yellow]⚠ Verification encountered issues[/yellow]")
        console.print(f"[dim]{e}[/dim]")
    except Exception as e:
        console.print(f"[yellow]⚠ Verification check error: {e}[/yellow]")


def perform_basic_checks(script_name: str):
    """Perform basic checks when script is not available"""
    import shutil

    if script_name == "verify-system.sh":
        # Check for required tools
        tools = ["kubectl", "docker", "curl"]
        for tool in tools:
            if shutil.which(tool):
                console.print(f"[green]✓[/green] {tool} found")
            else:
                console.print(f"[red]✗[/red] {tool} not found")

    elif script_name == "verify-kubernetes.sh":
        # Check kubectl
        try:
            result = subprocess.run(
                ["kubectl", "version", "--client"],
                capture_output=True,
                text=True,
            )
            if result.returncode == 0:
                console.print("[green]✓[/green] kubectl is working")
            else:
                console.print("[red]✗[/red] kubectl failed")
        except Exception as e:
            console.print(f"[red]✗[/red] kubectl check failed: {e}")

    elif script_name == "verify-guildnet.sh":
        # Check if GuildNet API is reachable
        try:
            import httpx

            response = httpx.get("https://localhost:8090/healthz", verify=False, timeout=5)
            if response.status_code == 200:
                console.print("[green]✓[/green] GuildNet API is reachable")
            else:
                console.print("[red]✗[/red] GuildNet API returned error")
        except Exception as e:
            console.print(f"[red]✗[/red] GuildNet API not reachable: {e}")

