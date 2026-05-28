# Project Map

**Purpose:** MCP server for persistent AI agent memory — SQLite-backed graph, FTS5 full-text search, runtime caching, and deferred writes. Zero-config, single binary.

## Notes for AI Agents

- **Entry points:** `main.go` (wire & serve), `store/tools.go` (MCP tool registration), `store/db.go` (schema & migration)
- **Main patterns:** In-memory `MemoryStore` wrapping `*sql.DB`; `runtime.Runtime` provides TTL caches, hot tracking, write queue; MCP tool handler adapts JSON args to store methods
- **General rule:** Read this file before proposing structural changes or modifying multiple modules. All writes go through `assertWritable()`; all mutations call `invalidateProject()` to bust caches.

---

## 1. Entry Point

Wires the database, runtime, and MCP server together. Exposes 16 tools over stdio.

```text
main.go
```

**Main responsibilities:**

- Parse `MEM_DB_PATH`, `MEM_READONLY`, `MEM_PROJECT_ID` from env
- Open SQLite via `store.OpenMemoryDb`
- Create `runtime.Runtime` and bind its `onFlush` to the DB
- Create `store.MemoryStore`, register MCP tools, serve over stdio

**Key files:**

- `main.go`: bootstrap, config, server lifecycle

**Relationships:**

- Depends on `store` and `runtime` packages
- Depends on `github.com/modelcontextprotocol/go-sdk/mcp`

---

## 2. Store — Core Memory Logic

Domain layer: CRUD for projects, sessions, events, decisions, lessons, plus knowledge graph and structured JSON queries.

```text
store/
├── db.go              # Connection, PRAGMAs, DDL schema, column migrations
├── memory.go          # MemoryStore: all business operations
├── tools.go           # MCP tool definitions and JSON arg parsing
├── helpers.go         # Scanning, redaction, scoring, normalization, UUID
├── graph_types.go     # Valid node types and relationship enums
├── graph_node.go      # Node upsert, get-by-ref, lookup, batch fetch
├── graph_edge.go      # Edge create, query, filter by direction/relationship
├── graph_link.go      # Link two nodes with type/relationship validation
├── graph_traverse.go  # Neighbors, file/feature trace, related lessons
├── graph_auto_link.go # Event-triggered auto-linking (files, blockers, features)
├── json_query.go      # JSON-criteria query engine for all entity types
├── json_query_test.go
└── memory_test.go     # Integration tests
```

**Main responsibilities:**

- Schema creation and lazy column migration (`db.go`)
- All business operations: startup context, sessions, events, decisions, lessons, stats, consolidation (`memory.go`)
- MCP tool registration: 16 tools with input schemas and argument coercion (`tools.go`)
- Knowledge graph: typed nodes and edges, auto-linking from events/decisions, trace queries (`graph_*.go`)
- Structured query with filters, `OR`/`NOT`, pagination (`json_query.go`)
- Secret redaction, FTS query construction, lesson scoring, tag overlap (`helpers.go`)

**Key files:**

- `store/db.go:48-196`: entire DDL and migration logic
- `store/memory.go:28-836`: `MemoryStore` struct and all public methods
- `store/tools.go:10-155`: all 16 MCP tool registrations
- `store/helpers.go:149-155`: `redact()` — strips API keys, tokens, secrets
- `store/graph_types.go`: valid node types (9) and relationships (9)
- `store/json_query.go`: `QueryCriteria` struct and SQL builder

**Relationships:**

- Depends on `runtime` package for caching, hot lessons, write queue
- Depends on `modernc.org/sqlite` (embedded SQLite)
- Exposed via MCP through `tools.go`

---

## 3. Runtime — Caching & Cognition Layer

In-memory speed layer: TTL caches, hot lesson tracking, deferred write batching, session working context, snapshots.

```text
runtime/
├── runtime.go    # Runtime struct, config, cache accessors, shutdown
├── cache.go      # Generic TTL cache with hit/miss stats, cleanup goroutine
├── hot.go        # Hot lessons tracker: scored, sorted, size-bounded
├── queue.go      # Deferred write queue with batching and retry
├── session.go    # Per-session WorkingContext (recent actions, summary)
├── snapshot.go   # SHA-256-keyed snapshot storage for startup contexts
└── runtime_test.go
```

**Main responsibilities:**

- `Runtime` orchestrates all sub-components (`runtime.go`)
- `TTLCache[K,V]` provides generic TTL caching with predicate invalidation (`cache.go`)
- `HotTracker` keeps top-N lessons by recency-weighted confidence + occurrences score (`hot.go`)
- `WriteOp` queue batches non-critical writes (e.g., `last_used_at` updates) with configurable flush interval and retries (`queue.go`)
- `WorkingContext` tracks per-session recent actions (for context-aware responses) (`session.go`)
- Snapshots store serialized startup context by hash for cross-reference (`snapshot.go`)

**Key files:**

- `runtime/runtime.go:30-39`: `DefaultConfig` with all TTLs and limits
- `runtime/cache.go:14-22`: generic `TTLCache` with atomic hit/miss counters
- `runtime/hot.go:96-110`: `computeScore()` — confidence × occurrences with recency bonuses
- `runtime/queue.go:44-70`: `flushLoop()` — batch flushing with ticker and size threshold

**Relationships:**

- Used exclusively by `store.MemoryStore` for non-critical performance concerns
- `onFlush` callback bridges back to `*sql.DB` for actual writes

---

## 4. SQLite Data Model

Six core tables, one FTS5 virtual table, indices and triggers.

| Table                | Purpose                                                              |
| -------------------- | -------------------------------------------------------------------- |
| `projects`           | Workspace scope (SHA-256 hash of root path)                          |
| `sessions`           | One per chat/work turn; status: active/closed/abandoned              |
| `events`             | Typed occurrences (note, progress, file_changed, blocked, etc.)      |
| `decisions`          | Technical choices with rationale, alternatives, evidence, confidence |
| `reasoning_memories` | Reusable lessons; scored, reinforced, with FTS5 index                |
| `memory_graph_nodes` | Typed graph entities (Feature, File, Blocker, etc.)                  |
| `memory_graph_edges` | Typed relationships (WORKED_ON, DERIVED_FROM, etc.)                  |

**Key files:**

- `store/db.go:49-171`: full DDL with constraints, indexes, FTS5 + triggers
- `store/db.go:198-226`: `ensureColumn()` lazy migration helper

---

## 5. Configuration

Environment-only, no config file.

| Variable         | Default             | Purpose                     |
| ---------------- | ------------------- | --------------------------- |
| `MEM_DB_PATH`    | `.memory/memory.db` | SQLite database path        |
| `MEM_PROJECT_ID` | auto (SHA-256[:16]) | Fixed project identifier    |
| `MEM_READONLY`   | `false`             | Blocks all write operations |
