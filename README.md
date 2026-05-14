# mem

Simple STDIO MCP server for compact, auditable project memory, backed by SQLite.

It stores lightweight project memory (sessions, events, decisions, and lessons). It does not manage tasks.

## Quickstart

```powershell
npm install
npm run build
npm test
```

To start the server manually over STDIO:

```powershell
node build/index.js
```

Important: when running over STDIO, do not write logs to stdout. This server uses stderr for operational messages.

## Requirements

- Node.js 24 or higher.
- npm.

Note: `better-sqlite3` is the only native dependency.

## Configuration

Example for a client compatible with `mcpServers`:

```json
{
  "mcpServers": {
    "memory": {
      "command": "node",
      "args": ["<ABSOLUTE_PATH_TO_REPO>/build/index.js"],
      "env": {
        "MEM_DB_PATH": "<ABSOLUTE_PATH_TO_REPO>/.memory/memory.db"
      }
    }
  }
}
```

On Windows, you can use `/` paths as shown above or escape `\\` inside JSON.

## Environment Variables

- `MEM_DB_PATH`: SQLite path. Default: `.memory/memory.db`.
- `MEM_PROJECT_ID`: fixed project id. Default: hash of the resolved `project_path`.
- `MEM_READONLY`: `true`, `1`, or `yes` to block writes.

## What It Stores

The model is intentionally small and project-agnostic:

- `project`: workspace scope.
- `session`: one chat or work turn that produced memory.
- `event`: relevant note or evidence-backed occurrence.
- `decision`: durable technical or project decision.
- `lesson`: reusable knowledge that should survive across sessions.

Anything that controls work state belongs outside this MCP, in your project documentation.

## Recommended Flow

1. `memory_get_startup_context` with `project_path` to create or retrieve project memory.
2. `memory_start_session` to open a memory session.
3. `memory_log_event` for relevant notes, checks, evidence, or outcomes.
4. `memory_log_decision` for decisions whose rationale should persist.
5. `memory_add_lesson` only for reusable lessons tied to the source session.
6. `memory_reinforce_lesson` when an existing lesson proves useful again.
7. `memory_close_session` at the end of the chat/work turn.

Optional: pass `namespace` to group related sessions (kebab-case segments, max 3 levels), for example `auth/jwt`, `build/ci`, `memory/search`, `auth/jwt/refresh`.

## Examples

Startup context input:

```json
{
  "project_path": "<ABSOLUTE_OR_RELATIVE_PROJECT_PATH>",
  "response_format": "concise"
}
```

Start session with namespace:

```json
{
  "project_path": "<ABSOLUTE_OR_RELATIVE_PROJECT_PATH>",
  "agent_name": "opencode",
  "namespace": "memory/search"
}
```

Log an event:

```json
{
  "session_id": "<session-id>",
  "kind": "docs_checked",
  "content": "Confirmed that the memory MCP should not control work state.",
  "evidence_refs": ["README.md"]
}
```

Log a decision:

```json
{
  "session_id": "<session-id>",
  "decision": "Keep mem focused on memory, not task management.",
  "rationale": "Workflow state and verification rules belong in project documentation.",
  "evidence_refs": ["README.md"],
  "confidence": "high"
}
```

Lesson search (FTS5 keyword search + tag boosts):

```json
{
  "project_id": "<project-id>",
  "query": "failing test",
  "tags": ["tests"],
  "limit": 5,
  "response_format": "concise"
}
```

## Session Lifecycle

A memory session represents one active chat or work turn. Close every session when the current turn ends unless the next turn should intentionally reuse it with `continue_existing: true`.

Current behavior:

- `active` sessions become `closed`.
- `closed_at` is set automatically.
- `summary` is optional but recommended.
- Closed sessions cannot be used for further writes.
- `memory_start_session` with `continue_existing: true` only reuses active sessions.

Example session close:

```json
{
  "session_id": "<session-id>",
  "summary": "Recorded memory decisions and reusable lessons."
}
```

## Tools

- `memory_get_startup_context`
- `memory_start_session`
- `memory_close_session`
- `memory_log_event`
- `memory_log_decision`
- `memory_search_lessons`
- `memory_add_lesson`
- `memory_reinforce_lesson`
- `memory_get_details`
- `memory_consolidate_lessons`

## Key Rules

- No free-form SQL is exposed.
- STDIO does not use stdout logs; errors go to stderr.
- The MCP does not create, start, close, approve, or validate work items.
- `rationale` and `evidence_refs` are stored separately.
- Lessons must be tied to a source session.
- Reinforce repeated lessons instead of adding duplicates.
- Responses are compact by default; use details only when needed.

## Data Location

By default, memory is stored in `.memory/memory.db` (relative to the working directory), or in `MEM_DB_PATH` if configured.
