# REST API Mock for mem MCP Tools

> HTTP wrapper around `store.MemoryStore` for integration testing of the mem MCP server without an MCP transport.

## Goal

Provide a standalone HTTP server that exposes every mem MCP tool as a REST endpoint, backed by the real `store.MemoryStore` with a temp SQLite database. Enables testing via `curl`, HTTP clients, or Go `httptest` without running the MCP stdio server.

## Endpoints

| Method | Path | MCP Tool | 
|--------|------|----------|
| `GET` | `/health` | n/a |
| `POST` | `/startup-context` | `get_startup_context` |
| `POST` | `/session/start` | `start_session` |
| `POST` | `/session/close` | `close_session` |
| `POST` | `/event` | `log_event` |
| `POST` | `/decision` | `log_decision` |
| `POST` | `/lesson/add` | `add_lesson` |
| `POST` | `/lesson/search` | `search_lessons` |
| `POST` | `/lesson/reinforce` | `reinforce_lesson` |
| `POST` | `/lesson/consolidate` | `consolidate_lessons` |
| `POST` | `/details` | `get_details` |
| `POST` | `/stats` | `stats` |

## Request/Response

All endpoints accept `application/json` POST bodies mirroring the MCP tool argument schemas. Responses are `application/json` with either `{"ok": true, "result": {...}}` or `{"ok": false, "error": "..."}`.

## Design Decisions

1. **Real store, ephemeral database** — uses `store.OpenMemoryDb` on `t.TempDir()` (or `MEM_DB_PATH` env var), not mocked. Tests exercise the real persistence layer.
2. **No runtime layer** — the REST mock skips the runtime cache since it's not needed for HTTP request/response testing.
3. **Project auto-init** — the first request to any tool endpoint that needs a project_id auto-initializes a project from the configured base path.

## Files

- `main.go` — entry point; wires up the store and starts the HTTP server
- `server.go` — HTTP handler registration, request parsing, error formatting
- `server_test.go` — integration tests covering the full flow from startup to close
