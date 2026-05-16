# File Watcher PRD

## Overview
A file system watcher utility for the mem project that monitors a directory tree for file system events (create, modify, delete) and reports them as structured events. Designed as a test utility to validate that the memory system correctly handles file-level change events.

## Requirements
- Watch a target directory recursively for file changes
- Detect file creation, modification, and deletion
- Support filtering by file extension
- Report events with file path, event type, and timestamp
- Run as a polling-based watcher (no external dependencies)
- Provide debounced event emission to coalesce rapid changes

## Events
- `file_created` – new file detected
- `file_modified` – existing file content changed
- `file_deleted` – file removed from disk

## Decisions
- Polling interval: 500ms default, configurable
- Debounce window: 200ms for modify events on same path
- Hash-based change detection (sha256 of file contents)

## Lessons
- Polling-based watching is simpler and more portable than OS-level notification APIs
- Debouncing is essential to avoid flooding consumers with duplicate modify events during writes
