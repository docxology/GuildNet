#!/usr/bin/env python3
"""MetaGuildNet CLI - Main entry point"""

import os
import sys
from pathlib import Path

import click
from rich.console import Console

from metaguildnet.api.client import Client
from metaguildnet.config.manager import ConfigManager

console = Console()


@click.group()
@click.option("--api-url", envvar="MGN_API_URL", help="GuildNet API URL")
@click.option("--token", envvar="MGN_API_TOKEN", help="API authentication token")
@click.option("--config", type=click.Path(), help="Config file path")
@click.option("-v", "--verbose", is_flag=True, help="Verbose output")
@click.pass_context
def cli(ctx, api_url, token, config, verbose):
    """MetaGuildNet CLI - GuildNet management utilities"""
    ctx.ensure_object(dict)

    # Load configuration
    config_path = Path(config) if config else Path.home() / ".metaguildnet" / "config.yaml"
    config_manager = ConfigManager(config_path)
    cfg = config_manager.load()

    # Override with command-line options
    if api_url:
        cfg["api"]["base_url"] = api_url
    if token:
        cfg["api"]["token"] = token

    ctx.obj["config"] = cfg
    ctx.obj["verbose"] = verbose
    ctx.obj["client"] = Client(
        base_url=cfg.get("api", {}).get("base_url", "https://localhost:8090"),
        token=cfg.get("api", {}).get("token", ""),
    )


@cli.command()
def version():
    """Show version information"""
    from metaguildnet import __version__

    console.print(f"MetaGuildNet CLI version {__version__}")


# Import subcommands
from metaguildnet.cli import cluster, database, install, verify, viz, workspace

cli.add_command(cluster.cluster)
cli.add_command(workspace.workspace)
cli.add_command(database.db)
cli.add_command(install.install)
cli.add_command(verify.verify)
cli.add_command(viz.viz)


if __name__ == "__main__":
    cli()

