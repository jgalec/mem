import json
import tempfile
from pathlib import Path

import pytest

from .syncer import (
    ConfigSyncError,
    backup_file,
    deep_merge,
    diff_configs,
    load_json,
    restore_backup,
    save_json,
    sync_file,
)


class TestDeepMerge:
    def test_overwrite_scalar(self):
        assert deep_merge({"a": 1}, {"a": 2}) == {"a": 2}

    def test_nested_merge(self):
        base = {"mcpServers": {"memory": {"command": "/old/path"}}}
        overlay = {"mcpServers": {"memory": {"command": "/new/path"}, "other": {"enabled": True}}}
        result = deep_merge(base, overlay)
        assert result["mcpServers"]["memory"]["command"] == "/new/path"
        assert result["mcpServers"]["other"]["enabled"] is True

    def test_add_new_keys(self):
        assert deep_merge({"a": 1}, {"b": 2}) == {"a": 1, "b": 2}

    def test_empty_base(self):
        assert deep_merge({}, {"a": 1}) == {"a": 1}

    def test_empty_overlay(self):
        assert deep_merge({"a": 1}, {}) == {"a": 1}


class TestDiffConfigs:
    def test_added_key(self):
        changes = diff_configs({"newKey": 42}, {})
        assert changes["newKey"]["action"] == "add"

    def test_updated_key(self):
        changes = diff_configs({"key": "new"}, {"key": "old"})
        assert changes["key"]["action"] == "update"
        assert changes["key"]["old"] == "old"
        assert changes["key"]["new"] == "new"

    def test_no_changes(self):
        changes = diff_configs({"a": 1}, {"a": 1})
        assert changes == {}

    def test_nested_diff(self):
        template = {"srv": {"mem": {"cmd": "/new"}}}
        target = {"srv": {"mem": {"cmd": "/old"}}}
        changes = diff_configs(template, target)
        assert changes["srv"]["action"] == "merge"
        assert changes["srv"]["value"]["mem"]["action"] == "update"


class TestFileIO:
    def test_save_and_load_json(self):
        data = {"key": "value"}
        with tempfile.NamedTemporaryFile(suffix=".json", delete=False, mode="w", encoding="utf-8") as f:
            path = Path(f.name)
            f.close()
            save_json(path, data)
            loaded = load_json(path)
            assert loaded == data
            path.unlink()

    def test_load_nonexistent(self):
        with pytest.raises(ConfigSyncError):
            load_json(Path("/nonexistent/config.json"))


class TestBackup:
    def test_backup_creates_timestamped_copy(self):
        data = {"x": 1}
        with tempfile.NamedTemporaryFile(suffix=".json", delete=False, mode="w", encoding="utf-8") as f:
            path = Path(f.name)
            f.close()
            save_json(path, data)
            backup = backup_file(path)
            assert backup.exists()
            assert load_json(backup) == data
            path.unlink()
            backup.unlink()


class TestSyncFile:
    def test_sync_creates_new_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            template = Path(tmp) / "template.json"
            target = Path(tmp) / "target.json"
            save_json(template, {"version": 1})
            result = sync_file(template, target)
            assert target.exists()
            assert load_json(target) == {"version": 1}
            assert result["changes"]["file"]["action"] == "create"

    def test_sync_dry_run_does_not_write(self):
        with tempfile.TemporaryDirectory() as tmp:
            template = Path(tmp) / "template.json"
            target = Path(tmp) / "target.json"
            save_json(template, {"version": 1})
            save_json(target, {"version": 2})
            result = sync_file(template, target, dry_run=True)
            assert result["dry_run"] is True
            assert load_json(target) == {"version": 2}

    def test_sync_backs_up_existing(self):
        with tempfile.TemporaryDirectory() as tmp:
            template = Path(tmp) / "template.json"
            target = Path(tmp) / "target.json"
            save_json(template, {"a": 1, "b": 2})
            save_json(target, {"b": 99})
            result = sync_file(template, target)
            assert result["backed_up"] is True
            assert load_json(target) == {"b": 99, "a": 1}


class TestRestore:
    def test_restore_from_backup(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "config.json"
            save_json(target, {"original": True})
            bk = backup_file(target)
            save_json(target, {"modified": True})
            restored_from = restore_backup(target)
            assert restored_from == bk
            assert load_json(target) == {"original": True}

    def test_restore_no_backup_raises(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "no_backup.json"
            save_json(target, {})
            with pytest.raises(ConfigSyncError, match="No backup found"):
                restore_backup(target)
