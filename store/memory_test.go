package store

import (
	"fmt"
	"strings"
	"testing"
)

func newFixture(t *testing.T) *MemoryStore {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenMemoryDb(dir + "/memory.db")
	if err != nil {
		t.Fatalf("OpenMemoryDb: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewMemoryStore(db, MemoryStoreConfig{})
}

func TestStartupContextInitializesProjectMemoryCompactly(t *testing.T) {
	store := newFixture(t)
	ctx, err := store.getStartupContext(t.TempDir(), "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	sessions := ctx["active_sessions"].([]map[string]interface{})
	decisions := ctx["recent_decisions"].([]map[string]interface{})
	lessons := ctx["relevant_lessons"].([]map[string]interface{})
	if len(sessions) != 0 {
		t.Errorf("expected 0 active sessions, got %d", len(sessions))
	}
	if len(decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(decisions))
	}
	if len(lessons) != 0 {
		t.Errorf("expected 0 lessons, got %d", len(lessons))
	}
	nextStep, ok := ctx["next_step"].(string)
	if !ok || !strings.Contains(nextStep, "Start a memory session") {
		t.Errorf("expected next_step to contain 'Start a memory session', got %v", nextStep)
	}
}

func TestStartSessionCreatesNewSessionsAndCanContinue(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	first, err := store.startSession(dir, ptr("test"), ptr("Auth / JWT / Refresh"), false)
	if err != nil {
		t.Fatalf("startSession 1: %v", err)
	}
	second, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession 2: %v", err)
	}

	fid := first["session"].(map[string]interface{})["id"].(string)
	sid := second["session"].(map[string]interface{})["id"].(string)
	if fid == sid {
		t.Error("session IDs should differ")
	}
	if first["continued"] != false {
		t.Error("first.continued should be false")
	}
	if second["continued"] != false {
		t.Error("second.continued should be false")
	}
	if first["session"].(map[string]interface{})["namespace"] != "auth/jwt/refresh" {
		t.Errorf("namespace mismatch: %v", first["session"].(map[string]interface{})["namespace"])
	}

	continued, err := store.startSession(dir, ptr("test"), nil, true)
	if err != nil {
		t.Fatalf("continueExisting: %v", err)
	}
	if continued["session"].(map[string]interface{})["id"] != sid {
		t.Error("continued session should reuse most recent active")
	}
	if continued["continued"] != true {
		t.Error("continued should be true")
	}
}

func TestLogEventRecordsSessionScopedMemory(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	session, err := store.startSession(dir, nil, nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)

	result, err := store.logEvent(sid, "docs_checked", "Read architecture notes", []string{"docs/architecture.md"})
	if err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	event := result["event"].(map[string]interface{})
	if event["kind"] != "docs_checked" {
		t.Errorf("kind: %v", event["kind"])
	}
	if event["content"] != "Read architecture notes" {
		t.Errorf("content: %v", event["content"])
	}
	refs, ok := event["evidence_refs"].([]string)
	if !ok || len(refs) != 1 || refs[0] != "docs/architecture.md" {
		t.Errorf("evidence_refs: %v", event["evidence_refs"])
	}
}

func TestDecisionsKeepRationaleSeparateFromEvidenceRefs(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	session, err := store.startSession(dir, nil, nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)

	rationale := "Local and auditable"
	result, err := store.logDecision(sid, "Use SQLite", &rationale, nil, []string{"prd-mem-development.md"}, "high")
	if err != nil {
		t.Fatalf("logDecision: %v", err)
	}
	decision := result["decision"].(map[string]interface{})
	if decision["rationale"] != "Local and auditable" {
		t.Errorf("rationale: %v", decision["rationale"])
	}
	refs, ok := decision["evidence_refs"].([]string)
	if !ok || len(refs) != 1 || refs[0] != "prd-mem-development.md" {
		t.Errorf("evidence_refs: %v", decision["evidence_refs"])
	}
}

func TestLessonSearchPrefersMatchingTagsAndQueryTerms(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	projectCtx, err := store.getStartupContext(dir, "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := projectCtx["project"].(map[string]interface{})["id"].(string)

	session, err := store.startSession(dir, nil, nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)

	outcome := "failure"
	_, err = store.addLesson(pid, "Reproduce before refactor", "Avoid broad edits", "Run the minimal failing test first", sid, &outcome, "consolidated", []string{"tests", "debugging"}, []string{"test run"})
	if err != nil {
		t.Fatalf("addLesson: %v", err)
	}

	found, err := store.searchLessons(pid, "failing test", []string{"tests"}, 3, "concise")
	if err != nil {
		t.Fatalf("searchLessons: %v", err)
	}
	lessons := found["lessons"].([]map[string]interface{})
	if len(lessons) != 1 {
		t.Fatalf("expected 1 lesson, got %d", len(lessons))
	}
	if lessons[0]["title"] != "Reproduce before refactor" {
		t.Errorf("title: %v", lessons[0]["title"])
	}
	if intVal(lessons[0], "occurrences") != 1 {
		t.Errorf("occurrences: %v", lessons[0]["occurrences"])
	}
	if floatVal(lessons[0], "confidence", 0) != 1.0 {
		t.Errorf("confidence: %v", lessons[0]["confidence"])
	}
}

func TestLessonsCanBeReinforcedWithoutDuplicating(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	projectCtx, err := store.getStartupContext(dir, "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := projectCtx["project"].(map[string]interface{})["id"].(string)

	session, err := store.startSession(dir, nil, nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)

	added, err := store.addLesson(pid, "Keep memory minimal", "Avoid unused complexity", "Reinforce repeated lessons instead of adding duplicates", sid, nil, "observed", []string{"memory"}, []string{"first"})
	if err != nil {
		t.Fatalf("addLesson: %v", err)
	}
	lesson := added["lesson"].(map[string]interface{})
	lessonId := lesson["id"].(int64)

	if intVal(lesson, "occurrences") != 1 {
		t.Errorf("initial occurrences: %v", lesson["occurrences"])
	}

	reinforced, err := store.reinforceLesson(lessonId, []string{"second"})
	if err != nil {
		t.Fatalf("reinforceLesson: %v", err)
	}
	r := reinforced["lesson"].(map[string]interface{})
	if intVal(r, "occurrences") != 2 {
		t.Errorf("occurrences after reinforce: %v", r["occurrences"])
	}
	if floatVal(r, "confidence", 0) != 1.05 {
		t.Errorf("confidence after reinforce: %v", r["confidence"])
	}
	refs, ok := r["evidence_refs"].([]string)
	if !ok || len(refs) != 2 || refs[0] != "first" || refs[1] != "second" {
		t.Errorf("evidence_refs: %v", r["evidence_refs"])
	}
	if r["last_reinforced_at"] == nil {
		t.Error("last_reinforced_at should be set")
	}
}

func TestLessonConsolidationSuggestsPromotionAndRelated(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	projectCtx, err := store.getStartupContext(dir, "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := projectCtx["project"].(map[string]interface{})["id"].(string)

	session, err := store.startSession(dir, nil, nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)

	first, err := store.addLesson(pid, "Small changes", "Prefer minimal edits", "Avoid broad rewrites", sid, nil, "observed", []string{"memory", "minimal"}, nil)
	if err != nil {
		t.Fatalf("addLesson 1: %v", err)
	}
	firstId := first["lesson"].(map[string]interface{})["id"].(int64)

	_, err = store.addLesson(pid, "Minimal memory", "Keep memory small", "Add only memory that changes future behavior", sid, nil, "observed", []string{"memory", "minimal"}, nil)
	if err != nil {
		t.Fatalf("addLesson 2: %v", err)
	}
	_, err = store.addLesson(pid, "Prefers minimal changes", "Same intent different title", "Favor small edits across codebase", sid, nil, "observed", []string{"memory", "minimal"}, nil)
	if err != nil {
		t.Fatalf("addLesson 3: %v", err)
	}

	dupResult, err := store.addLesson(pid, "  small   changes ", "Duplicate title", "Auto-reinforce should catch this", sid, nil, "observed", []string{"cleanup"}, nil)
	if err != nil {
		t.Fatalf("addLesson 4: %v", err)
	}
	if dupResult["auto_reinforced"] != true {
		t.Error("expected auto_reinforced for duplicate title")
	}

	rawJSON := "[]"
	store.db.Exec(
		"INSERT INTO reasoning_memories (project_id, title, description, content, source_session_id, source_outcome, status, tags, confidence, occurrences, evidence_refs, failure_mode, created_at, last_reinforced_at, last_used_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1.0, 1, ?, NULL, ?, NULL, NULL)",
		pid, "small changes", "dup", "dup", sid, nil, "observed", rawJSON, rawJSON, isoNow(),
	)

	store.reinforceLesson(firstId, nil)
	store.reinforceLesson(firstId, nil)

	result, err := store.consolidateLessons(pid, true)
	if err != nil {
		t.Fatalf("consolidateLessons: %v", err)
	}
	suggestions := result["suggestions"].([]map[string]interface{})

	hasMerge := false
	hasInspect := false
	hasPromote := false
	for _, s := range suggestions {
		switch s["action"] {
		case "merge_or_archive_duplicate":
			hasMerge = true
		case "inspect_related_lessons":
			hasInspect = true
		case "promote_to_consolidated":
			if s["lesson_id"] == firstId {
				hasPromote = true
			}
		}
	}
	if !hasMerge {
		t.Error("missing merge_or_archive_duplicate suggestion")
	}
	if !hasInspect {
		t.Error("missing inspect_related_lessons suggestion")
	}
	if !hasPromote {
		t.Error("missing promote_to_consolidated for first lesson")
	}
}

func TestCloseSessionClosesAndRejectsWritesToClosed(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	session, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)
	if session["session"].(map[string]interface{})["status"] != "active" {
		t.Error("initial status should be active")
	}

	summary := "Recorded memory updates"
	result, err := store.closeSession(sid, &summary)
	if err != nil {
		t.Fatalf("closeSession: %v", err)
	}
	closed := result["session"].(map[string]interface{})
	if closed["status"] != "closed" {
		t.Errorf("status: %v", closed["status"])
	}
	if closed["closed_at"] == nil {
		t.Error("closed_at should be set")
	}
	if closed["summary"] != "Recorded memory updates" {
		t.Errorf("summary: %v", closed["summary"])
	}

	_, err = store.logEvent(sid, "note", "late write", nil)
	if err == nil {
		t.Error("expected error for closed session write")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("error should mention closed: %v", err)
	}
}

func TestReadonlyModeRejectsWrites(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenMemoryDb(dir + "/memory.db")
	if err != nil {
		t.Fatalf("OpenMemoryDb: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store := NewMemoryStore(db, MemoryStoreConfig{Readonly: true})

	_, err = store.startSession(dir, nil, nil, false)
	if err == nil {
		t.Error("expected readonly error")
	}
	if !strings.Contains(err.Error(), "readonly mode") {
		t.Errorf("error should mention readonly: %v", err)
	}
}

func TestWorkControlEntityTypesAreNotPartOfMemoryDetails(t *testing.T) {
	store := newFixture(t)

	_, err := store.getDetails("feature", 1)
	if err == nil {
		t.Error("feature should fail")
	}
	_, err = store.getDetails("blocker", 1)
	if err == nil {
		t.Error("blocker should fail")
	}
	_, err = store.getDetails("review", 1)
	if err == nil {
		t.Error("review should fail")
	}
}

// graphInit returns a seeded store with project and session (no namespace).
func graphInit(t *testing.T) (*MemoryStore, string, string) {
	t.Helper()
	store := newFixture(t)
	dir := t.TempDir()
	ctx, err := store.getStartupContext(dir, "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := ctx["project"].(map[string]interface{})["id"].(string)
	session, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)
	return store, pid, sid
}

func TestGraphLinkCreatesNodesAndEdges(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	ctx, err := store.getStartupContext(dir, "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := ctx["project"].(map[string]interface{})["id"].(string)

	result, err := store.graphLink(pid, "Decision", "Use SQLite", nil, "File", "db/schema.sql", nil, "SUPPORTED_BY", nil, nil, nil)
	if err != nil {
		t.Fatalf("graphLink: %v", err)
	}
	if result["status"] != "linked" {
		t.Errorf("expected status linked, got %v", result["status"])
	}
	from := result["from"].(map[string]interface{})
	to := result["to"].(map[string]interface{})
	if from["type"] != "Decision" || from["label"] != "Use SQLite" {
		t.Errorf("from node: type=%v label=%v", from["type"], from["label"])
	}
	if to["type"] != "File" || to["label"] != "db/schema.sql" {
		t.Errorf("to node: type=%v label=%v", to["type"], to["label"])
	}
	edge := result["edge"].(map[string]interface{})
	if edge["relationship"] != "SUPPORTED_BY" {
		t.Errorf("edge relationship: %v", edge["relationship"])
	}
}

func TestGraphNeighborsReturnsBothDirections(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	ctx, err := store.getStartupContext(dir, "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := ctx["project"].(map[string]interface{})["id"].(string)

	link, err := store.graphLink(pid, "Feature", "auth", nil, "File", "auth/login.go", nil, "TOUCHED", nil, nil, nil)
	if err != nil {
		t.Fatalf("graphLink 1: %v", err)
	}
	toId := int64(intVal(link["to"].(map[string]interface{}), "id"))

	neighbors, err := store.graphNeighbors(toId, "both", nil)
	if err != nil {
		t.Fatalf("graphNeighbors: %v", err)
	}
	if neighbors["direction"] != "both" {
		t.Errorf("direction: %v", neighbors["direction"])
	}
	nodes := neighbors["neighbors"].([]map[string]interface{})
	if len(nodes) != 1 {
		t.Errorf("expected 1 neighbor, got %d", len(nodes))
	}
	if nodes[0]["type"] != "Feature" {
		t.Errorf("neighbor type: %v", nodes[0]["type"])
	}
}

func TestGraphTraceFileLinksFeaturesAndEvidence(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	ctx, err := store.getStartupContext(dir, "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := ctx["project"].(map[string]interface{})["id"].(string)

	store.graphLink(pid, "Feature", "api", nil, "File", "server.go", nil, "TOUCHED", nil, nil, nil)
	store.graphLink(pid, "Evidence", "profiling result", nil, "File", "server.go", nil, "SUPPORTED_BY", nil, nil, nil)

	trace, err := store.graphTraceFile(pid, ptr("server.go"), nil)
	if err != nil {
		t.Fatalf("graphTraceFile: %v", err)
	}
	features := trace["features"].([]map[string]interface{})
	if len(features) != 1 || features[0]["label"] != "api" {
		t.Errorf("features: %v", features)
	}
	evidence := trace["evidence"].([]map[string]interface{})
	if len(evidence) != 1 || evidence[0]["label"] != "profiling result" {
		t.Errorf("evidence: %v", evidence)
	}
}

func TestGraphFindRelatedLessonsViaDerivedFrom(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	ctx, err := store.getStartupContext(dir, "concise")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := ctx["project"].(map[string]interface{})["id"].(string)

	store.graphLink(pid, "Lesson", "Keep it simple", nil, "Feature", "refactor", nil, "DERIVED_FROM", nil, nil, nil)

	result, err := store.graphFindRelatedLessons(pid, ptr("refactor"), nil, nil)
	if err != nil {
		t.Fatalf("graphFindRelatedLessons: %v", err)
	}
	lessons := result["lessons"].([]map[string]interface{})
	if len(lessons) != 1 || lessons[0]["label"] != "Keep it simple" {
		t.Errorf("lessons: %v", lessons)
	}
}

func TestGraphAutoLinkOnLogEventFileChanged(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	session, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)
	pid := session["session"].(map[string]interface{})["project_id"].(string)

	_, err = store.logEvent(sid, "file_changed", "edited src/main.go", []string{"src/main.go"})
	if err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	fileNode, err := store.graphFindFileNode(pid, "src/main.go")
	if err != nil {
		t.Fatalf("graphFindFileNode: %v", err)
	}
	if fileNode["type"] != "File" || fileNode["label"] != "src/main.go" {
		t.Errorf("file node: type=%v label=%v", fileNode["type"], fileNode["label"])
	}
}

func TestGraphAutoLinkOnLogDecisionCreatesDecisionNode(t *testing.T) {
	store, _, sid := graphInit(t)

	result, err := store.logDecision(sid, "Adopt WAL mode", nil, nil, []string{"store/db.go"}, "high")
	if err != nil {
		t.Fatalf("logDecision: %v", err)
	}
	decisionId := intVal(result["decision"].(map[string]interface{}), "id")

	node, err := store.queryRow(
		"SELECT * FROM memory_graph_nodes WHERE type = 'Decision' AND entity_ref = ?",
		fmt.Sprintf("decision:%d", decisionId),
	)
	if err != nil || node == nil {
		t.Fatal("expected Decision graph node after logDecision")
	}
	if node["type"] != "Decision" || !strings.Contains(strVal(node, "label"), "Adopt WAL mode") {
		t.Errorf("decision node: type=%v label=%v", node["type"], node["label"])
	}
}

func TestGraphAutoLinkOnAddLessonCreatesLessonNode(t *testing.T) {
	store, pid, sid := graphInit(t)

	result, err := store.addLesson(pid, "Test before commit", "Always run tests", "gates quality", sid, nil, "observed", []string{"testing", "ci"}, nil)
	if err != nil {
		t.Fatalf("addLesson: %v", err)
	}
	lessonId := intVal(result["lesson"].(map[string]interface{}), "id")

	node, err := store.queryRow(
		"SELECT * FROM memory_graph_nodes WHERE type = 'Lesson' AND entity_ref = ?",
		fmt.Sprintf("lesson:%d", lessonId),
	)
	if err != nil || node == nil {
		t.Fatal("expected Lesson graph node after addLesson")
	}
	if node["type"] != "Lesson" || node["label"] != "Test before commit" {
		t.Errorf("lesson node: type=%v label=%v", node["type"], node["label"])
	}
}

func TestFullMemoryFlowIntegration(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	ctx, err := store.getStartupContext(dir, "detailed")
	if err != nil {
		t.Fatalf("getStartupContext: %v", err)
	}
	pid := ctx["project"].(map[string]interface{})["id"].(string)

	session, err := store.startSession(dir, ptr("test-agent"), ptr("auth/jwt"), false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := session["session"].(map[string]interface{})["id"].(string)

	_, err = store.logEvent(sid, "file_changed", "updated auth/middleware.go for token validation", []string{"auth/middleware.go"})
	if err != nil {
		t.Fatalf("logEvent file_changed: %v", err)
	}
	_, err = store.logEvent(sid, "test_run", "ran auth unit tests", []string{"auth/middleware_test.go"})
	if err != nil {
		t.Fatalf("logEvent test_run: %v", err)
	}

	rationale := "Stateless and auditable"
	decision, err := store.logDecision(sid, "Use JWT over sessions", &rationale, nil, []string{"auth/middleware.go"}, "high")
	if err != nil {
		t.Fatalf("logDecision: %v", err)
	}
	did := decision["decision"].(map[string]interface{})["id"].(int64)

	lesson, err := store.addLesson(pid, "Validate tokens early", "Check JWT before hitting handlers", "Middleware chain should reject invalid tokens at the edge", sid, ptr("success"), "consolidated", []string{"auth", "middleware"}, []string{"auth/middleware.go"})
	if err != nil {
		t.Fatalf("addLesson: %v", err)
	}
	lid := lesson["lesson"].(map[string]interface{})["id"].(int64)

	reinforced, err := store.reinforceLesson(lid, []string{"auth/middleware_test.go"})
	if err != nil {
		t.Fatalf("reinforceLesson: %v", err)
	}
	if intVal(reinforced["lesson"].(map[string]interface{}), "occurrences") != 2 {
		t.Error("occurrences should be 2 after reinforce")
	}

	decisionDetail, err := store.getDetails("decision", fmt.Sprintf("%d", did))
	if err != nil {
		t.Fatalf("getDetails decision: %v", err)
	}
	if decisionDetail["rationale"] != "Stateless and auditable" {
		t.Errorf("decision rationale: %v", decisionDetail["rationale"])
	}

	lessonDetail, err := store.getDetails("lesson", fmt.Sprintf("%d", lid))
	if err != nil {
		t.Fatalf("getDetails lesson: %v", err)
	}
	if lessonDetail["title"] != "Validate tokens early" {
		t.Errorf("lesson title: %v", lessonDetail["title"])
	}

	sessionDetail, err := store.getDetails("session", sid)
	if err != nil {
		t.Fatalf("getDetails session: %v", err)
	}
	if sessionDetail["status"] != "active" {
		t.Errorf("session status: %v", sessionDetail["status"])
	}

	search, err := store.searchLessons(pid, "JWT token", []string{"auth"}, 3, "detailed")
	if err != nil {
		t.Fatalf("searchLessons: %v", err)
	}
	lessons := search["lessons"].([]map[string]interface{})
	if len(lessons) == 0 {
		t.Fatal("expected at least 1 lesson in search results")
	}

	consolidation, err := store.consolidateLessons(pid, true)
	if err != nil {
		t.Fatalf("consolidateLessons: %v", err)
	}
	if consolidation["suggestions"] == nil {
		t.Error("expected suggestions key in consolidation result")
	}

	summary := "Implemented JWT auth with early token validation"
	closed, err := store.closeSession(sid, &summary)
	if err != nil {
		t.Fatalf("closeSession: %v", err)
	}
	if closed["session"].(map[string]interface{})["status"] != "closed" {
		t.Error("session should be closed")
	}

	finalCtx, err := store.getStartupContext(dir, "detailed")
	if err != nil {
		t.Fatalf("final getStartupContext: %v", err)
	}
	activeSessions := finalCtx["active_sessions"].([]map[string]interface{})
	if len(activeSessions) != 0 {
		t.Error("no active sessions expected after close")
	}
	recentDecisions := finalCtx["recent_decisions"].([]map[string]interface{})
	if len(recentDecisions) == 0 {
		t.Error("expected recent decisions in startup context")
	}
	relevantLessons := finalCtx["relevant_lessons"].([]map[string]interface{})
	if len(relevantLessons) == 0 {
		t.Error("expected relevant lessons in startup context")
	}

	if finalCtx["graph_context"] == nil {
		t.Error("expected graph_context in startup context")
	}
	gc := finalCtx["graph_context"].(map[string]interface{})
	if files, ok := gc["recent_files"].([]map[string]interface{}); ok {
		found := false
		for _, f := range files {
			if f["label"] == "auth/middleware.go" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected auth/middleware.go in graph_context recent_files")
		}
	}
}

func ptr(s string) *string {
	return &s
}
