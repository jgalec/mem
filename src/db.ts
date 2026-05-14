import Database from "better-sqlite3";
import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";

export type Db = Database.Database;

export function openMemoryDb(dbPath: string): Db {
  const resolved = resolve(dbPath);
  mkdirSync(dirname(resolved), { recursive: true });
  const db = new Database(resolved);
  db.pragma("foreign_keys = ON");
  migrate(db);
  return db;
}

export function migrate(db: Db): void {
  db.exec(`
CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  root_path TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  agent_name TEXT,
  namespace TEXT,
  status TEXT NOT NULL CHECK (status IN ('active', 'closed', 'abandoned')),
  summary TEXT,
  started_at TEXT NOT NULL,
  closed_at TEXT,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  session_id TEXT,
  kind TEXT NOT NULL CHECK (kind IN ('note', 'progress', 'tool_run', 'file_changed', 'test_run', 'docs_checked', 'blocked', 'closed')),
  content TEXT NOT NULL,
  evidence_refs TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE TABLE IF NOT EXISTS decisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  session_id TEXT,
  decision TEXT NOT NULL,
  rationale TEXT,
  alternatives_considered TEXT,
  evidence_refs TEXT,
  confidence TEXT CHECK (confidence IN ('low', 'medium', 'high')),
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE TABLE IF NOT EXISTS reasoning_memories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  content TEXT NOT NULL,
  source_session_id TEXT,
  source_outcome TEXT CHECK (source_outcome IN ('success', 'failure')),
  status TEXT NOT NULL CHECK (status IN ('observed', 'hypothesis', 'consolidated', 'archived')),
  tags TEXT,
  confidence REAL DEFAULT 1.0,
  occurrences INTEGER NOT NULL DEFAULT 1,
  evidence_refs TEXT,
  failure_mode TEXT,
  created_at TEXT NOT NULL,
  last_reinforced_at TEXT,
  last_used_at TEXT,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (source_session_id) REFERENCES sessions(id)
);

CREATE VIRTUAL TABLE IF NOT EXISTS reasoning_memories_fts USING fts5(
  project_id UNINDEXED,
  title,
  description,
  content,
  tags
);

CREATE TRIGGER IF NOT EXISTS reasoning_memories_ai AFTER INSERT ON reasoning_memories BEGIN
  INSERT INTO reasoning_memories_fts(rowid, project_id, title, description, content, tags)
  VALUES (new.id, new.project_id, new.title, new.description, new.content, COALESCE(new.tags, ''));
END;

CREATE TRIGGER IF NOT EXISTS reasoning_memories_ad AFTER DELETE ON reasoning_memories BEGIN
  DELETE FROM reasoning_memories_fts WHERE rowid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS reasoning_memories_au AFTER UPDATE ON reasoning_memories BEGIN
  DELETE FROM reasoning_memories_fts WHERE rowid = old.id;
  INSERT INTO reasoning_memories_fts(rowid, project_id, title, description, content, tags)
  VALUES (new.id, new.project_id, new.title, new.description, new.content, COALESCE(new.tags, ''));
END;
`);

  ensureColumn(db, "reasoning_memories", "occurrences", "INTEGER NOT NULL DEFAULT 1");
  ensureColumn(db, "reasoning_memories", "last_reinforced_at", "TEXT");
  ensureColumn(db, "sessions", "namespace", "TEXT");

  db.exec(`
INSERT INTO reasoning_memories_fts(rowid, project_id, title, description, content, tags)
SELECT rm.id, rm.project_id, rm.title, rm.description, rm.content, COALESCE(rm.tags, '')
FROM reasoning_memories rm
WHERE NOT EXISTS (SELECT 1 FROM reasoning_memories_fts f WHERE f.rowid = rm.id);
`);
}

function ensureColumn(db: Db, table: string, column: string, definition: string): void {
  const columns = db.prepare(`PRAGMA table_info(${table})`).all() as Array<{ name: string }>;
  if (!columns.some((item) => item.name === column)) db.exec(`ALTER TABLE ${table} ADD COLUMN ${column} ${definition}`);
}
