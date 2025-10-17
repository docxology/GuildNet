"""Database management commands"""

import json

import click
from rich.console import Console
from rich.table import Table

console = Console()


@click.group("db")
def db():
    """Manage databases"""
    pass


@db.command("list")
@click.argument("cluster_id")
@click.pass_context
def list_dbs(ctx, cluster_id):
    """List databases"""
    client = ctx.obj["client"]

    try:
        dbs = client.databases(cluster_id).list()

        table = Table(title=f"Databases in {cluster_id}")
        table.add_column("ID", style="cyan")
        table.add_column("Name", style="green")
        table.add_column("Description")

        for db in dbs:
            table.add_row(
                db.get("id", ""),
                db.get("name", ""),
                db.get("description", "")
            )

        console.print(table)

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@db.command("create")
@click.argument("cluster_id")
@click.argument("name")
@click.option("--description", default="", help="Database description")
@click.pass_context
def create_db(ctx, cluster_id, name, description):
    """Create a database"""
    client = ctx.obj["client"]

    try:
        db = client.databases(cluster_id).create(name, description)
        console.print(f"[green]✓[/green] Database created: {db.get('id')}")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@db.group("table")
def table():
    """Manage tables"""
    pass


@table.command("create")
@click.argument("cluster_id")
@click.argument("db_id")
@click.argument("table_name")
@click.option("--schema", required=True, help="Schema definition (name:type:flags,...)")
@click.option("--primary-key", default="id", help="Primary key column")
@click.pass_context
def create_table(ctx, cluster_id, db_id, table_name, schema, primary_key):
    """Create a table"""
    client = ctx.obj["client"]

    try:
        # Parse schema
        columns = []
        for col_def in schema.split(","):
            parts = col_def.strip().split(":")
            col = {"name": parts[0], "type": parts[1]}

            if len(parts) > 2:
                for flag in parts[2:]:
                    if flag == "required":
                        col["required"] = True
                    elif flag == "unique":
                        col["unique"] = True
                    elif flag == "indexed":
                        col["indexed"] = True

            columns.append(col)

        table_spec = {
            "name": table_name,
            "primaryKey": primary_key,
            "schema": columns,
        }

        client.databases(cluster_id).create_table(db_id, table_spec)
        console.print(f"[green]✓[/green] Table created: {table_name}")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@db.command("query")
@click.argument("cluster_id")
@click.argument("db_id")
@click.argument("table")
@click.option("--limit", default=100, help="Maximum rows to return")
@click.option("--format", type=click.Choice(["table", "json"]), default="table")
@click.pass_context
def query_table(ctx, cluster_id, db_id, table, limit, format):
    """Query table rows"""
    client = ctx.obj["client"]

    try:
        rows = client.databases(cluster_id).query(db_id, table, limit)

        if format == "json":
            console.print_json(data=rows)
        else:
            if not rows:
                console.print("No rows found")
                return

            # Create table with columns from first row
            tbl = Table(title=f"{table} ({len(rows)} rows)")
            for col in rows[0].keys():
                tbl.add_column(col, style="cyan")

            for row in rows:
                tbl.add_row(*[str(v) for v in row.values()])

            console.print(tbl)

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


@db.command("insert")
@click.argument("cluster_id")
@click.argument("db_id")
@click.argument("table")
@click.option("--data", required=True, help="JSON data to insert")
@click.pass_context
def insert_rows(ctx, cluster_id, db_id, table, data):
    """Insert rows"""
    client = ctx.obj["client"]

    try:
        rows = json.loads(data)
        if not isinstance(rows, list):
            rows = [rows]

        ids = client.databases(cluster_id).insert(db_id, table, rows)
        console.print(f"[green]✓[/green] Inserted {len(ids)} row(s)")

    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        raise click.Abort()


db.add_command(table)

