# config-syncer-py PRD

> Python utility to sync MCP agent configuration files across project workspaces.

## Overview

`config-syncer-py` keeps AI agent configuration files (e.g., `opencode.json`, MCP server settings) consistent across multiple project roots by syncing from a template source. It supports diff previews, safe backups, and selective merge strategies.

## Features

- **Sync from template** — push a canonical config to one or more target projects
- **Diff mode** — preview changes without applying them (`--dry-run`)
- **Smart merge** — merge template values into existing configs preserving unmatched keys
- **Backup before overwrite** — auto-create `.bak` copies with timestamps
- **CLI interface** — simple `config-syncer` entry point via `click`
- **Path globbing** — target multiple projects with glob patterns

## Files

| File | Purpose |
|------|---------|
| `__init__.py` | Package init, version export |
| `syncer.py` | Core logic: read, diff, merge, write |
| `cli.py` | Click-based CLI: `sync`, `diff`, `restore` commands |
| `test_syncer.py` | Pytest suite covering all paths |

## Usage

```
config-syncer sync --template template.json --target ./projects/*
config-syncer diff --template template.json --target ./my-project
config-syncer restore --target ./my-project
```

## Dependencies

- Python 3.10+
- `click` (CLI)
- `pytest` (testing)
- `deepdiff` (config diffing)
