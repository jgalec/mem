package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func OpenMemoryDb(dbPath string) (*sql.DB, error) {
	resolved, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("resolve db path: %w", err)
	}
	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite", resolved)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	ddl := `
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

CREATE TABLE IF NOT EXISTS memory_graph_nodes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('Project','Feature','Session','Decision','Evidence','Lesson','Blocker','File','Command')),
  entity_ref TEXT,
  label TEXT NOT NULL,
  metadata_json TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS memory_graph_edges (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  from_node_id INTEGER NOT NULL,
  to_node_id INTEGER NOT NULL,
  relationship TEXT NOT NULL,
  evidence_refs TEXT,
  source_session_id TEXT,
  source_entity TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (from_node_id) REFERENCES memory_graph_nodes(id),
  FOREIGN KEY (to_node_id) REFERENCES memory_graph_nodes(id)
);

CREATE INDEX IF NOT EXISTS idx_graph_nodes_project_type ON memory_graph_nodes(project_id, type);
CREATE INDEX IF NOT EXISTS idx_graph_nodes_entity_ref ON memory_graph_nodes(entity_ref);
CREATE INDEX IF NOT EXISTS idx_graph_edges_from ON memory_graph_edges(from_node_id);
CREATE INDEX IF NOT EXISTS idx_graph_edges_to ON memory_graph_edges(to_node_id);
CREATE INDEX IF NOT EXISTS idx_graph_edges_relationship ON memory_graph_edges(relationship);
`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	if err := ensureColumn(db, "reasoning_memories", "occurrences", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := ensureColumn(db, "reasoning_memories", "last_reinforced_at", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "sessions", "namespace", "TEXT"); err != nil {
		return err
	}

	_, err := db.Exec(`
INSERT INTO reasoning_memories_fts(rowid, project_id, title, description, content, tags)
SELECT rm.id, rm.project_id, rm.title, rm.description, rm.content, COALESCE(rm.tags, '')
FROM reasoning_memories rm
WHERE NOT EXISTS (SELECT 1 FROM reasoning_memories_fts f WHERE f.rowid = rm.id);
`)
	if err != nil {
		return fmt.Errorf("backfill fts: %w", err)
	}
	return nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("pragma table_info %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan pragma: %w", err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	if err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}
