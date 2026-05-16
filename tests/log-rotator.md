# Log Rotator PRD

## Overview

A Go package for rotating log files based on size thresholds. When a monitored log file exceeds a configurable maximum size, the rotator renames it with a sequential suffix (`.1`, `.2`, ...) and optionally compresses aged backups with gzip.

## Requirements

- **Size-based rotation**: Trigger rotation when file exceeds `MaxSize` bytes
- **Backup retention**: Keep at most `MaxBackups` rotated log files; oldest are deleted
- **Optional compression**: gzip old log files after rotation
- **Thread safety**: Safe for concurrent use from multiple goroutines
- **Idempotency**: Calling `Rotate()` on a file below the threshold is a no-op

## Files

| File | Purpose |
|------|---------|
| `rotator.go` | Core `Rotator` type, `Config`, rotation logic |
| `rotator_test.go` | Unit tests for rotation, backup cleanup, compression |

## API

```go
type Config struct {
    MaxSize    int64 // Rotate when file exceeds this size (bytes). 0 = no rotation.
    MaxBackups int   // Max rotated files to retain. 0 = keep all.
    Compress   bool  // Gzip old log files after rotation.
}

func New(cfg Config) *Rotator
func (r *Rotator) Rotate(path string) error
```

## Behavior

1. Check if file size > `MaxSize`
2. If not, return nil (no-op)
3. Remove oldest backup if needed (e.g., `app.log.3` when `MaxBackups=3`)
4. Shift existing backups: `app.log.2` → `app.log.3`, `app.log.1` → `app.log.2`
5. Rename current log: `app.log` → `app.log.1`
6. Optionally compress rotated files with gzip
7. Create empty `app.log` for continued writing
