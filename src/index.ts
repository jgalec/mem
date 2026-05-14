#!/usr/bin/env node
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";
import { openMemoryDb } from "./db.js";
import { MemoryError, MemoryStore } from "./memory.js";

const dbPath = process.env.MEM_DB_PATH ?? ".memory/memory.db";
const readonly = ["1", "true", "yes"].includes((process.env.MEM_READONLY ?? "false").toLowerCase());
const store = new MemoryStore(openMemoryDb(dbPath), { readonly, projectId: process.env.MEM_PROJECT_ID });

const server = new McpServer({ name: "mem", version: "0.1.0" });

const responseFormat = z.enum(["concise", "detailed"]).optional();
const evidenceRefs = z.array(z.string()).default([]);
const eventKind = z.enum(["note", "progress", "tool_run", "file_changed", "test_run", "docs_checked", "blocked", "closed"]).default("note");

function jsonResult(value: unknown) {
  return { content: [{ type: "text" as const, text: JSON.stringify(value, null, 2) }] };
}

function handle(action: () => unknown) {
  try {
    return jsonResult(action());
  } catch (error) {
    const message = error instanceof MemoryError ? error.message : `Unexpected mem error. Recommended action: inspect stderr logs. Detail: ${error instanceof Error ? error.message : String(error)}`;
    return { isError: true, content: [{ type: "text" as const, text: message }] };
  }
}

server.registerTool("memory_get_startup_context", {
  description: "Get compact project memory: active sessions, recent decisions, relevant lessons, and optional recent events.",
  inputSchema: { project_path: z.string(), response_format: responseFormat },
}, (input) => handle(() => store.getStartupContext(input)));

server.registerTool("memory_start_session", {
  description: "Create a memory session for a project. Pass continue_existing=true only when intentionally resuming the latest active session.",
  inputSchema: { project_path: z.string(), agent_name: z.string().optional(), namespace: z.string().optional(), continue_existing: z.boolean().default(false) },
}, (input) => handle(() => store.startSession(input)));

server.registerTool("memory_close_session", {
  description: "Close an active memory session. Optionally set a summary of what was remembered or accomplished.",
  inputSchema: { session_id: z.string(), summary: z.string().optional() },
}, (input) => handle(() => store.closeSession(input)));

server.registerTool("memory_log_event", {
  description: "Record a relevant memory event in the current session. This does not control work state.",
  inputSchema: { session_id: z.string(), kind: eventKind, content: z.string(), evidence_refs: evidenceRefs },
}, (input) => handle(() => store.logEvent(input)));

server.registerTool("memory_log_decision", {
  description: "Record a technical or project decision. Rationale and evidence_refs are stored separately.",
  inputSchema: { session_id: z.string(), decision: z.string(), rationale: z.string().optional(), alternatives_considered: z.array(z.string()).default([]), evidence_refs: evidenceRefs, confidence: z.enum(["low", "medium", "high"]).default("medium") },
}, (input) => handle(() => store.logDecision(input)));

server.registerTool("memory_search_lessons", {
  description: "Search strategic lessons with precision-oriented keyword and tag matching.",
  inputSchema: { project_id: z.string(), query: z.string().optional(), tags: z.array(z.string()).default([]), limit: z.number().int().positive().max(20).default(5), response_format: responseFormat },
}, (input) => handle(() => store.searchLessons(input)));

server.registerTool("memory_add_lesson", {
  description: "Store a strategic lesson tied to a source memory session.",
  inputSchema: { project_id: z.string(), title: z.string(), description: z.string(), content: z.string(), source_session_id: z.string(), source_outcome: z.enum(["success", "failure"]).optional(), status: z.enum(["observed", "hypothesis", "consolidated"]).default("observed"), tags: z.array(z.string()).default([]), evidence_refs: evidenceRefs },
}, (input) => handle(() => store.addLesson(input)));

server.registerTool("memory_reinforce_lesson", {
  description: "Reinforce an existing lesson when it proves useful again, without creating a duplicate.",
  inputSchema: { lesson_id: z.number().int().positive(), evidence_refs: evidenceRefs },
}, (input) => handle(() => store.reinforceLesson(input)));

server.registerTool("memory_get_details", {
  description: "Get details for one memory entity by id: event, decision, lesson, or session.",
  inputSchema: { entity_type: z.enum(["event", "decision", "lesson", "session"]), id: z.union([z.string(), z.number()]) },
}, (input) => handle(() => store.getDetails(input)));

server.registerTool("memory_consolidate_lessons", {
  description: "Suggest lesson consolidation actions. MVP is dry-run only.",
  inputSchema: { project_id: z.string(), dry_run: z.boolean().default(true) },
}, (input) => handle(() => store.consolidateLessons(input)));

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("mem running on stdio");
}

main().catch((error) => {
  console.error("Fatal error in mem:", error);
  process.exit(1);
});
