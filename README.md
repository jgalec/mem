# mem

> Compact, auditable project memory for AI coding agents — backed by SQLite, served over MCP.

**mem** is an [MCP](https://modelcontextprotocol.io/) server that gives AI coding agents persistent memory across sessions. It stores lessons, decisions, events, and session history in a local SQLite database, with full-text search, graph-aware context, and a runtime cognition layer that keeps hot memory fast.

---

## Features

- **Zero config** — just point it at a project path and start writing memory
- **Full-text search** — SQLite FTS5 with BM25 ranking for lesson retrieval
- **Graph layer** — auto-links sessions, features, files, decisions, blockers, and evidence
- **Runtime cognition** — hot lessons, retrieval cache, startup cache, and working context
- **Redaction** — automatically strips API keys, tokens, and secrets from stored content
- **Read-only mode** — safe read-only operation for inspection without writes
- **Single binary** — no dependencies at runtime beyond the OS

---

## Requirements

- [Go](https://go.dev/dl/) 1.26+
- SQLite (bundled via [modernc.org/sqlite](https://modernc.org/sqlite) — no CGO needed)

---

## Installation

```powershell
go install github.com/jgalec/mem@latest
```

Or build from source:

```powershell
git clone https://github.com/jgalec/mem.git
cd mem
go build -o build/mem.exe .
```

---

## Quickstart

Add mem to your MCP client configuration:

```json
{
  "mcpServers": {
    "memory": {
      "command": "<PATH_TO_REPO>/build/mem.exe",
      "env": {
        "MEM_DB_PATH": "<PATH_TO_REPO>/.memory/memory.db"
      }
    }
  }
}
```

On first use, your AI agent calls `mem_get_startup_context` with a `project_path`. mem creates the project and database automatically — no setup required.

---

## Recommended Flow

1. **`mem_get_startup_context`** — retrieve or initialize project memory
2. **`mem_start_session`** — open a new memory session (optionally with a `namespace`)
3. **`mem_log_event`** — record notes, file changes, test runs, or blockers
4. **`mem_log_decision`** — persist technical decisions with rationale
5. **`mem_search_lessons`** — find relevant lessons by keyword or tag
6. **`mem_add_lesson`** — store a reusable lesson tied to the source session
7. **`mem_reinforce_lesson`** — boost an existing lesson instead of duplicating
8. **`mem_close_session`** — close the session with an optional summary

---

## Tools

| Tool | Description |
|------|-------------|
| `mem_get_startup_context` | Get project snapshot: active sessions, decisions, lessons, graph context |
| `mem_start_session` | Create a memory session for a project |
| `mem_close_session` | Close an active session with optional summary |
| `mem_log_event` | Record an event (note, progress, file change, test run, blocker, etc.) |
| `mem_log_decision` | Log a technical decision with rationale and evidence |
| `mem_search_lessons` | Full-text search lessons with tag and status scoring |
| `mem_add_lesson` | Store a reusable strategic lesson (auto-reinforces on title match) |
| `mem_reinforce_lesson` | Boost an existing lesson's confidence and occurrences |
| `mem_get_details` | Retrieve full details for an event, decision, lesson, or session |
| `mem_consolidate_lessons` | Merge duplicates, promote lessons to consolidated (dry_run=false executes) |
| `mem_json_query` | Query entities with structured JSON criteria: filters, OR/NOT, pagination |
| `mem_stats` | Memory health: entity counts, graph size, cache hit rates, WAL status |
| `mem_list_sessions` | List sessions: filter by active/closed/all with counts |
| `mem_graph_trace_file` | Trace a file node to related features, evidence, lessons, commands |
| `mem_graph_trace_feature` | Trace a feature to its decisions, evidence, files, blockers, sessions |
| `mem_graph_find_related_lessons` | Find lessons related to a feature, file, or graph node |

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MEM_DB_PATH` | `.memory/memory.db` | Path to the SQLite database |
| `MEM_PROJECT_ID` | auto (SHA-256) | Fixed project identifier |
| `MEM_READONLY` | `false` | Set to `true`, `1`, or `yes` to block writes |

---

## Architecture

```
mem/
├── main.go              # Entry point, MCP server setup
├── store/               # Core memory logic
│   ├── db.go                # Database connection, migrations, schema
│   ├── memory.go            # MemoryStore: CRUD for all entity types
│   ├── helpers.go           # Scoring, redaction, normalization, utilities
│   ├── tools.go             # MCP tool registration and argument parsing
│   ├── graph_types.go       # Valid node types and relationships
│   ├── graph_node.go        # Node CRUD: upsert, get, resolve, find, fetch
│   ├── graph_edge.go        # Edge CRUD: create, query, filter by type
│   ├── graph_link.go        # Link and batch link with validation
│   ├── graph_traverse.go    # Traversal: neighbors, trace, related lessons
│   ├── graph_auto_link.go   # Auto-linking, startup context, path extraction
│   └── memory_test.go       # Integration tests
├── runtime/             # Hot memory and caching layer
│   ├── cache.go         # Generic TTL cache
│   ├── hot.go           # Hot lessons tracking
│   ├── queue.go         # Deferred write batching
│   ├── session.go       # Working context per session
│   ├── snapshot.go      # Startup context snapshots
│   └── runtime.go       # Runtime orchestration
└── build/               # Build output (gitignored)
```

### Data Model

| Entity | Purpose |
|--------|---------|
| **Project** | Workspace scope identified by root path |
| **Session** | One chat or work turn that produced memory |
| **Event** | Relevant note or evidence-backed occurrence |
| **Decision** | Durable technical or project decision |
| **Lesson** | Reusable knowledge that survives across sessions |
| **Graph Node** | Typed entity in the knowledge graph (Feature, File, Blocker, etc.) |
| **Graph Edge** | Typed relationship between nodes (WORKED_ON, TOUCHED, DERIVED_FROM, etc.) |

### Key Design Rules

- Memory does not control work state — that belongs in your project docs
- `rationale` and `evidence_refs` are stored separately from decisions
- Lessons must be tied to a source session
- Reinforce existing lessons instead of creating duplicates
- Secrets (API keys, tokens, passwords) are automatically redacted
- Compact responses by default; request `detailed` format only when needed

---

## Contributing

Pull requests are welcome. Please ensure tests pass before submitting:

```powershell
go test ./...
```

For major changes, open an issue first to discuss your proposal.

---

## License

[MIT](https://choosealicense.com/licenses/mit/)
