import type { Db } from "./db.js";
import { createHash, randomUUID } from "node:crypto";
import { basename, resolve } from "node:path";

type ResponseFormat = "concise" | "detailed";
type EventKind = "note" | "progress" | "tool_run" | "file_changed" | "test_run" | "docs_checked" | "blocked" | "closed";

type Project = { id: string; name: string; root_path: string; created_at: string; updated_at: string };
type Session = { id: string; project_id: string; agent_name: string | null; namespace: string | null; status: string; summary: string | null; started_at: string; closed_at: string | null };
type Lesson = { id: number; project_id: string; title: string; description: string; content: string; source_session_id: string | null; source_outcome: string | null; status: string; tags: string | null; confidence: number | null; occurrences: number; evidence_refs: string | null; failure_mode: string | null; created_at: string; last_reinforced_at: string | null; last_used_at: string | null };

export class MemoryError extends Error {}

export class MemoryStore {
  constructor(
    private readonly db: Db,
    private readonly options: { readonly: boolean; projectId?: string } = { readonly: false },
  ) {}

  getStartupContext(input: { project_path: string; response_format?: ResponseFormat }) {
    const project = this.ensureProject(input.project_path);
    const format = input.response_format ?? "concise";
    const activeSessions = this.db.prepare("SELECT id, agent_name, namespace, started_at FROM sessions WHERE project_id = ? AND status = 'active' ORDER BY started_at DESC LIMIT 3").all(project.id);
    const recentDecisions = this.db.prepare("SELECT id, decision, confidence, created_at FROM decisions WHERE project_id = ? ORDER BY id DESC LIMIT 5").all(project.id);
    const relevantLessons = this.db.prepare("SELECT id, title, description, status, tags, confidence, occurrences, last_reinforced_at FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY last_used_at DESC, occurrences DESC, id DESC LIMIT 5").all(project.id);
    const recentEvents = format === "detailed" ? this.db.prepare("SELECT id, session_id, kind, content, created_at FROM events WHERE project_id = ? ORDER BY id DESC LIMIT 10").all(project.id) : undefined;

    return {
      project: { id: project.id, name: project.name, root_path: project.root_path },
      active_sessions: activeSessions,
      recent_decisions: recentDecisions,
      relevant_lessons: relevantLessons,
      ...(recentEvents ? { recent_events: recentEvents } : {}),
      next_step: activeSessions.length > 0 ? "Continue with an active session or start a new one if this is a separate work turn." : "Start a memory session before writing events, decisions, or lessons.",
    };
  }

  startSession(input: { project_path: string; agent_name?: string; namespace?: string; continue_existing?: boolean }) {
    this.assertWritable();
    const project = this.ensureProject(input.project_path);
    if (input.continue_existing) {
      const active = this.db.prepare("SELECT * FROM sessions WHERE project_id = ? AND status = 'active' ORDER BY started_at DESC LIMIT 1").get(project.id) as Session | undefined;
      if (active) return { session: active, continued: true };
    }

    const now = isoNow();
    const session: Session = { id: randomUUID(), project_id: project.id, agent_name: input.agent_name ?? null, namespace: normalizeNamespace(input.namespace) ?? null, status: "active", summary: null, started_at: now, closed_at: null };
    this.db
      .prepare("INSERT INTO sessions (id, project_id, agent_name, namespace, status, summary, started_at, closed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
      .run(session.id, session.project_id, session.agent_name, session.namespace, session.status, session.summary, session.started_at, session.closed_at);
    return { session, continued: false };
  }

  closeSession(input: { session_id: string; summary?: string }) {
    this.assertWritable();
    const session = this.requireSession(input.session_id);
    const now = isoNow();
    this.db.prepare("UPDATE sessions SET status = 'closed', closed_at = ?, summary = ? WHERE id = ?").run(now, nullableRedact(input.summary), session.id);
    this.insertEvent(session.project_id, session.id, "closed", redact(input.summary ?? "Session closed."), []);
    return { session: this.getDetails({ entity_type: "session", id: session.id }), note: "Session closed. Start a new session for further memory writes." };
  }

  logEvent(input: { session_id: string; kind?: EventKind; content: string; evidence_refs?: string[] }) {
    this.assertWritable();
    const session = this.requireSession(input.session_id);
    const id = this.insertEvent(session.project_id, session.id, input.kind ?? "note", redact(input.content), input.evidence_refs ?? []);
    return { event: this.getDetails({ entity_type: "event", id }) };
  }

  logDecision(input: { session_id: string; decision: string; rationale?: string; alternatives_considered?: string[]; evidence_refs?: string[]; confidence?: "low" | "medium" | "high" }) {
    this.assertWritable();
    const session = this.requireSession(input.session_id);
    const now = isoNow();
    const result = this.db.prepare("INSERT INTO decisions (project_id, session_id, decision, rationale, alternatives_considered, evidence_refs, confidence, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)").run(session.project_id, session.id, redact(input.decision), nullableRedact(input.rationale), json(input.alternatives_considered ?? []), json(redactList(input.evidence_refs ?? [])), input.confidence ?? "medium", now);
    return { decision: this.getDetails({ entity_type: "decision", id: Number(result.lastInsertRowid) }), note: "rationale is stored separately from evidence_refs." };
  }

  searchLessons(input: { project_id: string; query?: string; tags?: string[]; limit?: number; response_format?: ResponseFormat }) {
    const limit = Math.min(Math.max(input.limit ?? 5, 1), 20);
    const query = (input.query ?? "").trim().toLowerCase();
    const tags = input.tags ?? [];
    const ftsQuery = query ? toFtsQuery(query) : "";
    let rows: Lesson[] = [];
    if (query) {
      rows = this.db
        .prepare(
          "SELECT rm.* FROM reasoning_memories_fts f JOIN reasoning_memories rm ON rm.id = f.rowid WHERE f.project_id = ? AND reasoning_memories_fts MATCH ? AND rm.status != 'archived' ORDER BY bm25(reasoning_memories_fts) ASC LIMIT 100",
        )
        .all(input.project_id, ftsQuery) as Lesson[];
      if (rows.length === 0) {
        rows = this.db.prepare("SELECT * FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY id DESC LIMIT 100").all(input.project_id) as Lesson[];
      }
    } else {
      rows = this.db.prepare("SELECT * FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY id DESC LIMIT 100").all(input.project_id) as Lesson[];
    }
    const scored = rows.map((lesson) => ({ lesson, score: lessonScore(lesson, query, tags) })).filter((item) => item.score > 0 || (!query && tags.length === 0)).sort((a, b) => b.score - a.score || b.lesson.occurrences - a.lesson.occurrences || b.lesson.id - a.lesson.id).slice(0, limit);
    const now = isoNow();
    const update = this.db.prepare("UPDATE reasoning_memories SET last_used_at = ? WHERE id = ?");
    for (const item of scored) update.run(now, item.lesson.id);
    return { lessons: scored.map(({ lesson, score }) => input.response_format === "detailed" ? expandLesson(lesson, score) : compactLesson(lesson, score)) };
  }

  addLesson(input: { project_id: string; title: string; description: string; content: string; source_session_id: string; source_outcome?: "success" | "failure"; status?: "observed" | "hypothesis" | "consolidated"; tags?: string[]; evidence_refs?: string[] }) {
    this.assertWritable();
    const project = this.db.prepare("SELECT * FROM projects WHERE id = ?").get(input.project_id) as Project | undefined;
    if (!project) throw new MemoryError(`Cannot add lesson because project '${input.project_id}' does not exist. Recommended action: call memory_get_startup_context or memory_start_session for the project first.`);
    const session = this.db.prepare("SELECT * FROM sessions WHERE id = ?").get(input.source_session_id) as Session | undefined;
    if (!session || session.project_id !== project.id) throw new MemoryError("Cannot add lesson without a source_session_id from the same project. Recommended action: start a memory session and use it as the lesson source.");
    const now = isoNow();
    const result = this.db.prepare("INSERT INTO reasoning_memories (project_id, title, description, content, source_session_id, source_outcome, status, tags, confidence, occurrences, evidence_refs, failure_mode, created_at, last_reinforced_at, last_used_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1.0, 1, ?, NULL, ?, NULL, NULL)").run(input.project_id, redact(input.title), redact(input.description), redact(input.content), input.source_session_id, input.source_outcome ?? null, input.status ?? "observed", json(input.tags ?? []), json(redactList(input.evidence_refs ?? [])), now);
    return { lesson: this.getDetails({ entity_type: "lesson", id: Number(result.lastInsertRowid) }) };
  }

  reinforceLesson(input: { lesson_id: number; evidence_refs?: string[] }) {
    this.assertWritable();
    const lesson = this.getDetails({ entity_type: "lesson", id: input.lesson_id }) as Lesson & { evidence_refs: string[] };
    const now = isoNow();
    const evidence = mergeStrings(lesson.evidence_refs ?? [], redactList(input.evidence_refs ?? []));
    const confidence = Math.min((lesson.confidence ?? 1.0) + 0.05, 2.0);
    this.db.prepare("UPDATE reasoning_memories SET occurrences = occurrences + 1, confidence = ?, evidence_refs = ?, last_reinforced_at = ?, last_used_at = ? WHERE id = ?").run(confidence, json(evidence), now, now, input.lesson_id);
    return { lesson: this.getDetails({ entity_type: "lesson", id: input.lesson_id }) };
  }

  getDetails(input: { entity_type: string; id: string | number }) {
    const tableByType: Record<string, string> = { event: "events", decision: "decisions", lesson: "reasoning_memories", session: "sessions" };
    const table = tableByType[input.entity_type];
    if (!table) throw new MemoryError(`Unknown entity_type '${input.entity_type}'. Recommended action: use event, decision, lesson, or session.`);
    const row = this.db.prepare(`SELECT * FROM ${table} WHERE id = ?`).get(input.id) as Record<string, unknown> | undefined;
    if (!row) throw new MemoryError(`No ${input.entity_type} found with id '${input.id}'. Recommended action: refresh startup context or search before requesting details.`);
    return hydrateJsonColumns(row);
  }

  consolidateLessons(input: { project_id: string; dry_run?: boolean }) {
    const lessons = this.db.prepare("SELECT id, title, description, status, tags, occurrences FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY id DESC").all(input.project_id) as Array<{ id: number; title: string; description: string; status: string; tags: string | null; occurrences: number }>;
    const suggestions = [];
    for (let i = 0; i < lessons.length; i++) {
      for (let j = i + 1; j < lessons.length; j++) {
        if (normalizeTitle(lessons[i]?.title) === normalizeTitle(lessons[j]?.title)) suggestions.push({ action: "merge_or_archive_duplicate", keep_id: lessons[i]?.id, candidate_id: lessons[j]?.id, reason: "same normalized title" });
        else if (tagOverlap(lessons[i]?.tags, lessons[j]?.tags) >= 2) suggestions.push({ action: "inspect_related_lessons", keep_id: lessons[i]?.id, candidate_id: lessons[j]?.id, reason: "shared tags" });
      }
      if ((lessons[i]?.occurrences ?? 0) >= 3 && lessons[i]?.status !== "consolidated") suggestions.push({ action: "promote_to_consolidated", lesson_id: lessons[i]?.id, reason: "reinforced at least 3 times" });
    }
    return { dry_run: input.dry_run ?? true, suggestions, note: "MVP returns suggestions only and does not modify data." };
  }

  private ensureProject(projectPath: string): Project {
    const root = resolve(projectPath);
    const id = this.options.projectId ?? createHash("sha256").update(root).digest("hex").slice(0, 16);
    const existing = this.db.prepare("SELECT * FROM projects WHERE id = ?").get(id) as Project | undefined;
    if (existing) return existing;
    this.assertWritable();
    const now = isoNow();
    const project: Project = { id, name: basename(root), root_path: root, created_at: now, updated_at: now };
    this.db.prepare("INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)").run(project.id, project.name, project.root_path, project.created_at, project.updated_at);
    return project;
  }

  private requireSession(sessionId: string): Session {
    const session = this.db.prepare("SELECT * FROM sessions WHERE id = ?").get(sessionId) as Session | undefined;
    if (!session) throw new MemoryError(`Cannot find session '${sessionId}'. Recommended action: call memory_start_session before writing memory.`);
    if (session.status !== "active") throw new MemoryError(`Cannot write to session '${sessionId}' because status is '${session.status}'. Recommended action: start a new session.`);
    return session;
  }

  private insertEvent(projectId: string, sessionId: string | null, kind: EventKind, content: string, evidenceRefs: string[]) {
    const result = this.db.prepare("INSERT INTO events (project_id, session_id, kind, content, evidence_refs, created_at) VALUES (?, ?, ?, ?, ?, ?)").run(projectId, sessionId, kind, content, json(redactList(evidenceRefs)), isoNow());
    return Number(result.lastInsertRowid);
  }

  private assertWritable() {
    if (this.options.readonly) throw new MemoryError("Mem is running in readonly mode. Recommended action: unset MEM_READONLY or use read-only tools only.");
  }
}

function isoNow() {
  return new Date().toISOString();
}

function json(value: unknown) {
  return JSON.stringify(value);
}

function parseJson(value: string | null) {
  if (!value) return [];
  try { return JSON.parse(value) as unknown; } catch { return value; }
}

function hydrateJsonColumns(row: Record<string, unknown>) {
  const copy = { ...row };
  for (const key of ["evidence_refs", "alternatives_considered", "tags"]) {
    if (typeof copy[key] === "string") copy[key] = parseJson(copy[key] as string);
  }
  return copy;
}

function expandLesson(lesson: Lesson, score: number) {
  return { ...hydrateJsonColumns(lesson as unknown as Record<string, unknown>), score };
}

function compactLesson(lesson: Lesson, score: number) {
  return { id: lesson.id, title: lesson.title, description: lesson.description, status: lesson.status, tags: parseJson(lesson.tags), confidence: lesson.confidence, occurrences: lesson.occurrences, last_reinforced_at: lesson.last_reinforced_at, score };
}

function lessonScore(lesson: Lesson, query: string, tags: string[]) {
  const haystack = `${lesson.title} ${lesson.description} ${lesson.content}`.toLowerCase();
  let score = query ? query.split(/\s+/).filter(Boolean).reduce((total, term) => total + (haystack.includes(term) ? 2 : 0), 0) : 1;
  const lessonTags = new Set((parseJson(lesson.tags) as string[]).map((tag) => tag.toLowerCase()));
  for (const tag of tags) if (lessonTags.has(tag.toLowerCase())) score += 3;
  if (lesson.status === "consolidated") score += 1;
  return score;
}

function nullableRedact(value?: string) {
  return value === undefined ? null : redact(value);
}

function redactList(values: string[]) {
  return values.map(redact);
}

function mergeStrings(first: string[], second: string[]) {
  return Array.from(new Set([...first, ...second]));
}

function tagOverlap(left: string | null | undefined, right: string | null | undefined) {
  const leftTags = new Set((parseJson(left ?? null) as string[]).map((tag) => tag.toLowerCase()));
  return (parseJson(right ?? null) as string[]).filter((tag) => leftTags.has(tag.toLowerCase())).length;
}

function normalizeTitle(value: string | null | undefined) {
  return (value ?? "").trim().replace(/\s+/g, " ").toLowerCase();
}

function normalizeNamespace(value: string | null | undefined) {
  if (!value) return null;
  const parts = value
    .trim()
    .split("/")
    .map((part) => part.trim())
    .filter(Boolean)
    .slice(0, 3)
    .map((part) => part
      .toLowerCase()
      .replace(/[^a-z0-9\s-]/g, "")
      .trim()
      .replace(/\s+/g, "-")
      .replace(/-+/g, "-")
      .replace(/^-+|-+$/g, ""))
    .filter(Boolean);
  const joined = parts.join("/");
  return joined.length > 0 ? joined.slice(0, 128) : null;
}

function toFtsQuery(query: string) {
  const terms = query
    .split(/\s+/)
    .map((term) => term.trim())
    .filter(Boolean)
    .map((term) => term.replaceAll('"', '""'))
    .map((term) => `"${term}"*`);
  return terms.length > 0 ? terms.join(" AND ") : "";
}

function redact(value: string) {
  return value
    .replace(/(api[_-]?key|token|secret|password)\s*[:=]\s*[^\s,;]+/gi, "$1=[REDACTED]")
    .replace(/\b(sk-[A-Za-z0-9_-]{12,}|ghp_[A-Za-z0-9_]{12,})\b/g, "[REDACTED]");
}
