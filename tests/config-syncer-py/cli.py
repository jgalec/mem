import sys
from pathlib import Path

import click

from .syncer import ConfigSyncError, restore_backup, sync_file


@click.group()
def main():
    """Sync AI agent config files across projects."""


@main.command()
@click.option("--template", required=True, type=click.Path(exists=True), help="Template config file")
@click.option("--target", required=True, type=click.Path(), help="Target config file or glob")
@click.option("--dry-run", is_flag=True, help="Preview changes without applying")
def sync(template, target, dry_run):
    """Sync a config from template to target(s)."""
    template_path = Path(template).resolve()
    target_path = Path(target).resolve()

    try:
        result = sync_file(template_path, target_path, dry_run=dry_run)
        click.echo(click.style("Sync result:", bold=True))
        if dry_run:
            click.echo(f"  [DRY RUN] Would sync {target_path}")
        for key, change in result["changes"].items():
            action = change.get("action", "?")
            click.echo(f"  {key}: {action}")
        if result.get("backed_up"):
            click.echo(f"  Backup created before overwrite")
    except ConfigSyncError as e:
        click.echo(click.style(f"Error: {e}", fg="red"), err=True)
        sys.exit(1)


@main.command()
@click.option("--template", required=True, type=click.Path(exists=True), help="Template config file")
@click.option("--target", required=True, type=click.Path(exists=True), help="Target config file")
def diff(template, target):
    """Show differences between template and target."""
    from .syncer import diff_configs, load_json

    template_data = load_json(Path(template).resolve())
    target_data = load_json(Path(target).resolve())

    changes = diff_configs(template_data, target_data)
    if not changes:
        click.echo("No differences found.")
    else:
        click.echo(click.style("Differences:", bold=True))
        for key, change in changes.items():
            action = change.get("action", "?")
            if action == "add":
                click.echo(f"  + {key}: {change['value']}")
            elif action == "update":
                click.echo(f"  ~ {key}: {change['old']} -> {change['new']}")
            elif action == "merge":
                click.echo(f"  M {key}: {change['value']}")


@main.command()
@click.option("--target", required=True, type=click.Path(exists=True), help="Config file to restore")
def restore(target):
    """Restore config from latest backup."""
    target_path = Path(target).resolve()
    try:
        backup = restore_backup(target_path)
        click.echo(f"Restored {target_path} from {backup}")
    except ConfigSyncError as e:
        click.echo(click.style(f"Error: {e}", fg="red"), err=True)
        sys.exit(1)


if __name__ == "__main__":
    main()
