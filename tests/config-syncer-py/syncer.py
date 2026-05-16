import json
import os
import shutil
from datetime import datetime, timezone
from pathlib import Path


class ConfigSyncError(Exception):
    pass


def load_json(path: Path) -> dict:
    try:
        with open(path, encoding="utf-8") as f:
            return json.load(f)
    except (json.JSONDecodeError, FileNotFoundError) as e:
        raise ConfigSyncError(f"Failed to read {path}: {e}")


def save_json(path: Path, data: dict) -> None:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)
        f.write("\n")


def deep_merge(base: dict, overlay: dict) -> dict:
    result = base.copy()
    for key, value in overlay.items():
        if key in result and isinstance(result[key], dict) and isinstance(value, dict):
            result[key] = deep_merge(result[key], value)
        else:
            result[key] = value
    return result


def diff_configs(template: dict, target: dict) -> dict:
    changes = {}
    for key, value in template.items():
        if key not in target:
            changes[key] = {"action": "add", "value": value}
        elif target[key] != value:
            if isinstance(value, dict) and isinstance(target[key], dict):
                nested = diff_configs(value, target[key])
                if nested:
                    changes[key] = {"action": "merge", "value": nested}
            else:
                changes[key] = {"action": "update", "old": target[key], "new": value}
    return changes


def backup_file(path: Path) -> Path:
    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    backup_path = path.with_suffix(f".bak-{ts}")
    shutil.copy2(path, backup_path)
    return backup_path


def sync_file(template_path: Path, target_path: Path, dry_run: bool = False) -> dict:
    template = load_json(template_path)

    if target_path.exists():
        target = load_json(target_path)
        changes = diff_configs(template, target)
    else:
        target = {}
        changes = {"file": {"action": "create", "value": template}}

    if dry_run:
        return {"target": str(target_path), "dry_run": True, "changes": changes}

    if target_path.exists() and changes:
        backup_file(target_path)

    merged = deep_merge(target, template)
    save_json(target_path, merged)
    return {"target": str(target_path), "backed_up": bool(changes), "changes": changes}


def restore_backup(target_path: Path) -> Path:
    directory = target_path.parent
    backups = sorted(
        directory.glob(f"{target_path.name}.bak-*"),
        key=os.path.getmtime,
        reverse=True,
    )
    if not backups:
        raise ConfigSyncError(f"No backup found for {target_path}")
    latest = backups[0]
    shutil.copy2(latest, target_path)
    return latest
