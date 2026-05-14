import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { openMemoryDb } from "./db.js";
import { MemoryError, MemoryStore } from "./memory.js";

function fixture() {
  const dir = mkdtempSync(join(tmpdir(), "mem-"));
  const db = openMemoryDb(join(dir, "memory.db"));
  const store = new MemoryStore(db);
  return { dir, db, store, cleanup: () => { db.close(); rmSync(dir, { recursive: true, force: true }); } };
}

test("startup context initializes project memory compactly", () => {
  const f = fixture();
  try {
    const context = f.store.getStartupContext({ project_path: f.dir });
    assert.equal(context.active_sessions.length, 0);
    assert.equal(context.recent_decisions.length, 0);
    assert.equal(context.relevant_lessons.length, 0);
    assert.match(context.next_step, /Start a memory session/);
  } finally {
    f.cleanup();
  }
});

test("startSession creates new sessions by default and can continue explicitly", () => {
  const f = fixture();
  try {
    const first = f.store.startSession({ project_path: f.dir, agent_name: "test", namespace: "Auth / JWT / Refresh" });
    const second = f.store.startSession({ project_path: f.dir, agent_name: "test" });
    assert.notEqual(first.session.id, second.session.id);
    assert.equal(first.continued, false);
    assert.equal(second.continued, false);
    assert.equal(first.session.namespace, "auth/jwt/refresh");

    const continued = f.store.startSession({ project_path: f.dir, agent_name: "test", continue_existing: true });
    assert.equal(continued.session.id, second.session.id);
    assert.equal(continued.continued, true);
  } finally {
    f.cleanup();
  }
});

test("logEvent records session-scoped memory without work control", () => {
  const f = fixture();
  try {
    const session = f.store.startSession({ project_path: f.dir }).session;
    const result = f.store.logEvent({ session_id: session.id, kind: "docs_checked", content: "Read architecture notes", evidence_refs: ["docs/architecture.md"] });
    assert.equal(result.event.kind, "docs_checked");
    assert.equal(result.event.content, "Read architecture notes");
    assert.deepEqual(result.event.evidence_refs, ["docs/architecture.md"]);
  } finally {
    f.cleanup();
  }
});

test("decisions keep rationale separate from evidence refs", () => {
  const f = fixture();
  try {
    const session = f.store.startSession({ project_path: f.dir }).session;
    const result = f.store.logDecision({ session_id: session.id, decision: "Use SQLite", rationale: "Local and auditable", evidence_refs: ["prd-mem-development.md"], confidence: "high" });
    assert.equal(result.decision.rationale, "Local and auditable");
    assert.deepEqual(result.decision.evidence_refs, ["prd-mem-development.md"]);
  } finally {
    f.cleanup();
  }
});

test("lesson search prefers matching tags and query terms", () => {
  const f = fixture();
  try {
    const project = f.store.getStartupContext({ project_path: f.dir }).project;
    const session = f.store.startSession({ project_path: f.dir }).session;
    f.store.addLesson({ project_id: project.id, title: "Reproduce before refactor", description: "Avoid broad edits", content: "Run the minimal failing test first", source_session_id: session.id, source_outcome: "failure", status: "consolidated", tags: ["tests", "debugging"], evidence_refs: ["test run"] });
    const found = f.store.searchLessons({ project_id: project.id, query: "failing test", tags: ["tests"], limit: 3 });
    assert.equal(found.lessons.length, 1);
    const first = found.lessons[0] as unknown as { title: string; occurrences: number; confidence: number } | undefined;
    assert.equal(first?.title, "Reproduce before refactor");
    assert.equal(first?.occurrences, 1);
    assert.equal(first?.confidence, 1);
  } finally {
    f.cleanup();
  }
});

test("lessons can be reinforced without duplicating them", () => {
  const f = fixture();
  try {
    const project = f.store.getStartupContext({ project_path: f.dir }).project;
    const session = f.store.startSession({ project_path: f.dir }).session;
    const added = f.store.addLesson({ project_id: project.id, title: "Keep memory minimal", description: "Avoid unused complexity", content: "Reinforce repeated lessons instead of adding duplicates", source_session_id: session.id, tags: ["memory"], evidence_refs: ["first"] });
    const lesson = added.lesson as { id: number; occurrences: number };
    assert.equal(lesson.occurrences, 1);

    const reinforced = f.store.reinforceLesson({ lesson_id: lesson.id, evidence_refs: ["second"] }).lesson as { occurrences: number; confidence: number; evidence_refs: string[]; last_reinforced_at: string | null };
    assert.equal(reinforced.occurrences, 2);
    assert.equal(reinforced.confidence, 1.05);
    assert.deepEqual(reinforced.evidence_refs, ["first", "second"]);
    assert.ok(reinforced.last_reinforced_at);
  } finally {
    f.cleanup();
  }
});

test("lesson consolidation suggests promotion and related lessons", () => {
  const f = fixture();
  try {
    const project = f.store.getStartupContext({ project_path: f.dir }).project;
    const session = f.store.startSession({ project_path: f.dir }).session;
    const first = f.store.addLesson({ project_id: project.id, title: "Small changes", description: "Prefer minimal edits", content: "Avoid broad rewrites", source_session_id: session.id, tags: ["memory", "minimal"] }).lesson as { id: number };
    f.store.addLesson({ project_id: project.id, title: "Minimal memory", description: "Keep memory small", content: "Add only memory that changes future behavior", source_session_id: session.id, tags: ["memory", "minimal"] });
    f.store.addLesson({ project_id: project.id, title: "  small   changes ", description: "Same title after normalization", content: "Normalize whitespace and case", source_session_id: session.id, tags: ["cleanup"] });
    f.store.reinforceLesson({ lesson_id: first.id });
    f.store.reinforceLesson({ lesson_id: first.id });

    const result = f.store.consolidateLessons({ project_id: project.id });
    assert.ok(result.suggestions.some((item) => item.action === "merge_or_archive_duplicate"));
    assert.ok(result.suggestions.some((item) => item.action === "inspect_related_lessons"));
    assert.ok(result.suggestions.some((item) => item.action === "promote_to_consolidated" && item.lesson_id === first.id));
  } finally {
    f.cleanup();
  }
});

test("closeSession closes an active session and rejects writes to closed sessions", () => {
  const f = fixture();
  try {
    const { session } = f.store.startSession({ project_path: f.dir, agent_name: "test" });
    assert.equal(session.status, "active");
    const result = f.store.closeSession({ session_id: session.id, summary: "Recorded memory updates" });
    assert.equal(result.session.status, "closed");
    assert.ok(result.session.closed_at);
    assert.equal(result.session.summary, "Recorded memory updates");
    assert.throws(() => f.store.logEvent({ session_id: session.id, content: "late write" }), /status is 'closed'/);
  } finally {
    f.cleanup();
  }
});

test("readonly mode rejects writes", () => {
  const f = fixture();
  try {
    const store = new MemoryStore(f.db, { readonly: true });
    assert.throws(() => store.startSession({ project_path: f.dir }), /readonly mode/);
  } finally {
    f.cleanup();
  }
});

test("work-control entity types are not part of memory details", () => {
  const f = fixture();
  try {
    assert.throws(() => f.store.getDetails({ entity_type: "feature", id: 1 }), /Unknown entity_type/);
    assert.throws(() => f.store.getDetails({ entity_type: "blocker", id: 1 }), /Unknown entity_type/);
    assert.throws(() => f.store.getDetails({ entity_type: "review", id: 1 }), /Unknown entity_type/);
  } finally {
    f.cleanup();
  }
});
