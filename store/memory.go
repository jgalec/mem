package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jgalec/mem/runtime"
)

type MemoryError struct{ msg string }

func (e *MemoryError) Error() string { return e.msg }

type MemoryStoreConfig struct {
	Readonly  bool
	ProjectId string
	Rt        *runtime.Runtime
}

type MemoryStore struct {
	db      *sql.DB
	readonly bool
	projectId string
	rt      *runtime.Runtime
}

func NewMemoryStore(db *sql.DB, cfg MemoryStoreConfig) *MemoryStore {
	return &MemoryStore{db: db, readonly: cfg.Readonly, projectId: cfg.ProjectId, rt: cfg.Rt}
}

func (s *MemoryStore) invalidateProject(pid string) {
	if s.rt != nil {
		s.rt.InvalidateStartupCache(pid)
		s.rt.InvalidateRetrievalCache(pid)
		s.rt.InvalidProjectSnapshots(pid)
	}
}

func (s *MemoryStore) getStartupContext(projectPath string, responseFormat string) (map[string]interface{}, error) {
	project, err := s.ensureProject(projectPath)
	if err != nil {
		return nil, err
	}
	if responseFormat == "" {
		responseFormat = "concise"
	}

	pid := project["id"].(string)
	cacheKey := "startup_context:" + pid + ":" + responseFormat

	if s.rt != nil {
		if cached, ok := s.rt.StartupCache().Get(cacheKey); ok {
			return cached, nil
		}
	}

	activeSessions, _ := s.queryRows("SELECT id, agent_name, namespace, started_at FROM sessions WHERE project_id = ? AND status = 'active' ORDER BY started_at DESC LIMIT 3", pid)
	recentDecisions, _ := s.queryRows("SELECT id, decision, confidence, created_at FROM decisions WHERE project_id = ? ORDER BY id DESC LIMIT 5", pid)
	relevantLessons, _ := s.queryRows("SELECT id, title, description, status, tags, confidence, occurrences, last_reinforced_at FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY last_used_at DESC, occurrences DESC, id DESC LIMIT 5", pid)

	result := map[string]interface{}{
		"project":          map[string]interface{}{"id": project["id"], "name": project["name"], "root_path": project["root_path"]},
		"active_sessions":  activeSessions,
		"recent_decisions": recentDecisions,
		"relevant_lessons": relevantLessons,
	}
	if graphCtx := s.graphBuildStartupContext(pid); graphCtx != nil {
		result["graph_context"] = graphCtx
	}

	if responseFormat == "detailed" {
		events, _ := s.queryRows("SELECT id, session_id, kind, content, created_at FROM events WHERE project_id = ? ORDER BY id DESC LIMIT 10", pid)
		result["recent_events"] = events
	}

	if len(activeSessions) > 0 {
		result["next_step"] = "Continue with an active session or start a new one if this is a separate work turn."
	} else {
		result["next_step"] = "Start a memory session before writing events, decisions, or lessons."
	}

	if s.rt != nil {
		s.rt.StartupCache().Set(cacheKey, result)
		snapshotKey, _ := s.rt.StoreSnapshot(pid, result)
		if snapshotKey != "" {
			result["snapshot_key"] = snapshotKey
		}
	}
	return result, nil
}

func (s *MemoryStore) startSession(projectPath string, agentName *string, namespace *string, continueExisting bool) (map[string]interface{}, error) {
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	project, err := s.ensureProject(projectPath)
	if err != nil {
		return nil, err
	}
	pid := project["id"].(string)

	if continueExisting {
		active, err := s.queryRow("SELECT * FROM sessions WHERE project_id = ? AND status = 'active' ORDER BY started_at DESC LIMIT 1", pid)
		if err == nil && active != nil {
			return map[string]interface{}{"session": active, "continued": true}, nil
		}
	}

	now := isoNow()
	id := newUUID()
	var ns interface{} = nil
	if namespace != nil {
		ns = normalizeNamespace(*namespace)
	}
	var ag interface{} = nil
	if agentName != nil {
		ag = *agentName
	}

	_, err = s.db.Exec(
		"INSERT INTO sessions (id, project_id, agent_name, namespace, status, summary, started_at, closed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, pid, ag, ns, "active", nil, now, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	session := map[string]interface{}{
		"id": id, "project_id": pid, "agent_name": ag, "namespace": ns,
		"status": "active", "summary": nil, "started_at": now, "closed_at": nil,
	}
	s.invalidateProject(pid)
	if s.rt != nil {
		s.rt.ClearSessionState(id)
		wc := &runtime.WorkingContext{
			Summary:  "Session started.",
			UpdatedAt: time.Now().UTC(),
		}
		s.rt.SetWorkingContext(id, wc)
	}
	s.graphAutoLinkFeatureFromNamespace(pid, id, namespace)
	return map[string]interface{}{"session": session, "continued": false}, nil
}

func (s *MemoryStore) closeSession(sessionId string, summary *string) (map[string]interface{}, error) {
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	session, err := s.requireSession(sessionId)
	if err != nil {
		return nil, err
	}
	if s.rt != nil {
		s.rt.DrainWriteQueue()
	}
	now := isoNow()

	var sum interface{} = nil
	rawSummary := "Session closed."
	if summary != nil {
		rawSummary = *summary
		sum = nullableRedact(*summary)
	}

	_, err = s.db.Exec("UPDATE sessions SET status = 'closed', closed_at = ?, summary = ? WHERE id = ?", now, sum, sessionId)
	if err != nil {
		return nil, fmt.Errorf("close session: %w", err)
	}

	s.insertEvent(session["project_id"].(string), sessionId, "closed", redact(rawSummary), nil)
	s.graphUpdateSessionSummary(sessionId, summary)

	details, _ := s.getDetails("session", sessionId)
	s.invalidateProject(session["project_id"].(string))
	if s.rt != nil {
		s.rt.ClearSessionState(sessionId)
	}
	s.checkpointWAL()
	return map[string]interface{}{
		"session": details,
		"note":    "Session closed. Start a new session for further memory writes.",
	}, nil
}

func (s *MemoryStore) logEvent(sessionId string, kind string, content string, evidenceRefs []string) (map[string]interface{}, error) {
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	session, err := s.requireSession(sessionId)
	if err != nil {
		return nil, err
	}
	if kind == "" {
		kind = "note"
	}
	id := s.insertEvent(session["project_id"].(string), sessionId, kind, redact(content), evidenceRefs)
	pid := session["project_id"].(string)
	s.invalidateProject(pid)
	if s.rt != nil {
		wc := s.rt.GetWorkingContext(sessionId)
		if wc != nil {
			wc.RecentActions = append(wc.RecentActions, kind)
			if len(wc.RecentActions) > 20 {
				wc.RecentActions = wc.RecentActions[len(wc.RecentActions)-20:]
			}
			s.rt.SetWorkingContext(sessionId, wc)
		}
	}
	switch kind {
	case "file_changed":
		s.graphAutoLinkFileChanges(pid, sessionId, content, evidenceRefs)
	case "blocked":
		s.graphAutoLinkBlocker(pid, sessionId, content)
	default:
		if len(evidenceRefs) > 0 {
			s.graphAutoLinkEvidence(pid, sessionId, id, content, evidenceRefs)
		}
	}
	details, _ := s.getDetails("event", id)
	return map[string]interface{}{"event": details}, nil
}

func (s *MemoryStore) logDecision(sessionId string, decision string, rationale *string, alternatives []string, evidenceRefs []string, confidence string) (map[string]interface{}, error) {
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	session, err := s.requireSession(sessionId)
	if err != nil {
		return nil, err
	}
	if confidence == "" {
		confidence = "medium"
	}
	now := isoNow()

	altJSON, _ := json.Marshal(alternatives)
	evJSON, _ := json.Marshal(redactList(evidenceRefs))

	var rat interface{} = nil
	if rationale != nil {
		rat = redact(*rationale)
	}

	result, err := s.db.Exec(
		"INSERT INTO decisions (project_id, session_id, decision, rationale, alternatives_considered, evidence_refs, confidence, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		session["project_id"], sessionId, redact(decision), rat, string(altJSON), string(evJSON), confidence, now,
	)
	if err != nil {
		return nil, fmt.Errorf("log decision: %w", err)
	}

	id, _ := result.LastInsertId()
	pid := session["project_id"].(string)
	s.graphAutoLinkDecision(pid, sessionId, id, decision, evidenceRefs)
	s.invalidateProject(pid)
	if s.rt != nil {
		wc := s.rt.GetWorkingContext(sessionId)
		if wc != nil {
			wc.RecentActions = append(wc.RecentActions, "decision:"+confidence)
			if len(wc.RecentActions) > 20 {
				wc.RecentActions = wc.RecentActions[len(wc.RecentActions)-20:]
			}
			s.rt.SetWorkingContext(sessionId, wc)
		}
	}
	details, _ := s.getDetails("decision", id)
	return map[string]interface{}{
		"decision": details,
		"note":     "rationale is stored separately from evidence_refs.",
	}, nil
}

func (s *MemoryStore) searchLessons(projectId string, query string, tags []string, limit int, responseFormat string) (map[string]interface{}, error) {
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	query = strings.TrimSpace(strings.ToLower(query))

	cacheKey := s.retrievalCacheKey(projectId, query, tags, limit, responseFormat)
	if s.rt != nil {
		if cached, ok := s.rt.RetrievalCache().Get(cacheKey); ok {
			lessons := make([]map[string]interface{}, len(cached))
			copy(lessons, cached)
			return map[string]interface{}{"lessons": lessons, "cached": true}, nil
		}
	}

	var rows []map[string]interface{}
	var err error
	if query != "" {
		ftsQuery := toFtsQuery(query)
		rows, err = s.queryRows(
			"SELECT rm.* FROM reasoning_memories_fts f JOIN reasoning_memories rm ON rm.id = f.rowid WHERE f.project_id = ? AND reasoning_memories_fts MATCH ? AND rm.status != 'archived' ORDER BY bm25(reasoning_memories_fts) ASC LIMIT 100",
			projectId, ftsQuery,
		)
		if err == nil && len(rows) == 0 {
			rows, _ = s.queryRows("SELECT * FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY id DESC LIMIT 100", projectId)
		}
		if err != nil {
			rows, _ = s.queryRows("SELECT * FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY id DESC LIMIT 100", projectId)
		}
	} else {
		rows, _ = s.queryRows("SELECT * FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY id DESC LIMIT 100", projectId)
	}

	type scoredItem struct {
		row   map[string]interface{}
		score int
	}
	var items []scoredItem
	for _, row := range rows {
		s := lessonScore(row, query, tags)
		if s > 0 || (query == "" && len(tags) == 0) {
			items = append(items, scoredItem{row, s})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		oi := intVal(items[i].row, "occurrences")
		oj := intVal(items[j].row, "occurrences")
		if oi != oj {
			return oi > oj
		}
		return intVal(items[i].row, "id") > intVal(items[j].row, "id")
	})
	if len(items) > limit {
		items = items[:limit]
	}

	now := isoNow()
	for _, item := range items {
		if s.rt != nil {
			s.rt.EnqueueWrite("UPDATE reasoning_memories SET last_used_at = ? WHERE id = ?", now, item.row["id"])
		} else {
			s.db.Exec("UPDATE reasoning_memories SET last_used_at = ? WHERE id = ?", now, item.row["id"])
		}
	}

	lessons := make([]map[string]interface{}, len(items))
	for i, item := range items {
		if responseFormat == "detailed" {
			lessons[i] = expandLesson(item.row, item.score)
		} else {
			lessons[i] = compactLesson(item.row, item.score)
		}
	}

	if s.rt != nil {
		for _, item := range items {
			s.rt.HotLessons().AddFromRow(item.row)
		}
		s.rt.RetrievalCache().Set(cacheKey, lessons)
	}
	return map[string]interface{}{"lessons": lessons}, nil
}

func (s *MemoryStore) addLesson(projectId string, title string, description string, content string, sourceSessionId string, sourceOutcome *string, status string, tags []string, evidenceRefs []string) (map[string]interface{}, error) {
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	project, err := s.queryRow("SELECT * FROM projects WHERE id = ?", projectId)
	if err != nil || project == nil {
		return nil, &MemoryError{"Cannot add lesson because project '" + projectId + "' does not exist. Recommended action: call memory_get_startup_context or memory_start_session for the project first."}
	}
	session, err := s.queryRow("SELECT * FROM sessions WHERE id = ?", sourceSessionId)
	if err != nil || session == nil || session["project_id"] != projectId {
		return nil, &MemoryError{"Cannot add lesson without a source_session_id from the same project. Recommended action: start a memory session and use it as the lesson source."}
	}

	normalizedTitle := normalizeTitle(title)
	existing, _ := s.queryRows(
		"SELECT id FROM reasoning_memories WHERE project_id = ? AND status != 'archived'",
		projectId,
	)
	for _, row := range existing {
		detail, err := s.getDetails("lesson", row["id"])
		if err == nil && normalizeTitle(strVal(detail, "title")) == normalizedTitle {
			result, err := s.reinforceLesson(int64(intVal(detail, "id")), evidenceRefs)
			if err != nil {
				return nil, err
			}
			result["auto_reinforced"] = true
			result["matched_title"] = strVal(detail, "title")
			return result, nil
		}
	}

	if status == "" {
		status = "observed"
	}

	var outcome interface{} = nil
	if sourceOutcome != nil {
		outcome = *sourceOutcome
	}

	tagsJSON, _ := json.Marshal(tags)
	evJSON, _ := json.Marshal(redactList(evidenceRefs))
	now := isoNow()

	result, err := s.db.Exec(
		"INSERT INTO reasoning_memories (project_id, title, description, content, source_session_id, source_outcome, status, tags, confidence, occurrences, evidence_refs, failure_mode, created_at, last_reinforced_at, last_used_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1.0, 1, ?, NULL, ?, NULL, NULL)",
		projectId, redact(title), redact(description), redact(content), sourceSessionId, outcome, status, string(tagsJSON), string(evJSON), now,
	)
	if err != nil {
		return nil, fmt.Errorf("add lesson: %w", err)
	}

	id, _ := result.LastInsertId()
	s.graphAutoLinkLesson(projectId, id, title, sourceSessionId, tags)
	s.invalidateProject(projectId)
	details, _ := s.getDetails("lesson", id)
	if s.rt != nil && details != nil {
		s.rt.HotLessons().AddFromRow(details)
	}
	return map[string]interface{}{"lesson": details}, nil
}

func (s *MemoryStore) reinforceLesson(lessonId int64, evidenceRefs []string) (map[string]interface{}, error) {
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	lesson, err := s.getDetails("lesson", lessonId)
	if err != nil {
		return nil, &MemoryError{"Cannot reinforce lesson: " + err.Error() + " Recommended action: check that the lesson exists."}
	}

	existing := toStringSlice(lesson, "evidence_refs")
	merged := mergeStrings(existing, redactList(evidenceRefs))
	conf := floatVal(lesson, "confidence", 1.0) + 0.05
	if conf > 2.0 {
		conf = 2.0
	}
	evJSON, _ := json.Marshal(merged)
	now := isoNow()

	_, err = s.db.Exec(
		"UPDATE reasoning_memories SET occurrences = occurrences + 1, confidence = ?, evidence_refs = ?, last_reinforced_at = ?, last_used_at = ? WHERE id = ?",
		conf, string(evJSON), now, now, lessonId,
	)
	if err != nil {
		return nil, fmt.Errorf("reinforce lesson: %w", err)
	}

	s.invalidateProject(lesson["project_id"].(string))
	details, _ := s.getDetails("lesson", lessonId)
	if s.rt != nil && details != nil {
		s.rt.HotLessons().AddFromRow(details)
	}
	return map[string]interface{}{"lesson": details}, nil
}

func (s *MemoryStore) getDetails(entityType string, id interface{}) (map[string]interface{}, error) {
	tableByType := map[string]string{
		"event":    "events",
		"decision": "decisions",
		"lesson":   "reasoning_memories",
		"session":  "sessions",
	}
	table, ok := tableByType[entityType]
	if !ok {
		return nil, &MemoryError{"Unknown entity_type '" + entityType + "'. Recommended action: use event, decision, lesson, or session."}
	}

	row, err := s.queryRow("SELECT * FROM "+table+" WHERE id = ?", id)
	if err != nil || row == nil {
		return nil, &MemoryError{fmt.Sprintf("No %s found with id '%v'. Recommended action: refresh startup context or search before requesting details.", entityType, id)}
	}
	return hydrateJsonColumns(row), nil
}

func (s *MemoryStore) consolidateLessons(projectId string, dryRun bool) (map[string]interface{}, error) {
	if dryRun == false {
		dryRun = true
	}
	rows, _ := s.queryRows("SELECT id, title, description, status, tags, occurrences FROM reasoning_memories WHERE project_id = ? AND status != 'archived' ORDER BY id DESC", projectId)

	var suggestions []map[string]interface{}
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if normalizeTitle(strVal(rows[i], "title")) == normalizeTitle(strVal(rows[j], "title")) {
				suggestions = append(suggestions, map[string]interface{}{
					"action":       "merge_or_archive_duplicate",
					"keep_id":      rows[i]["id"],
					"candidate_id": rows[j]["id"],
					"reason":       "same normalized title",
				})
			} else if tagOverlap(strVal(rows[i], "tags"), strVal(rows[j], "tags")) >= 2 {
				suggestions = append(suggestions, map[string]interface{}{
					"action":       "inspect_related_lessons",
					"keep_id":      rows[i]["id"],
					"candidate_id": rows[j]["id"],
					"reason":       "shared tags",
				})
			}
		}
		if intVal(rows[i], "occurrences") >= 3 && strVal(rows[i], "status") != "consolidated" {
			suggestions = append(suggestions, map[string]interface{}{
				"action":    "promote_to_consolidated",
				"lesson_id": rows[i]["id"],
				"reason":    "reinforced at least 3 times",
			})
		}
	}

	return map[string]interface{}{
		"dry_run":     dryRun,
		"suggestions": suggestions,
		"note":        "MVP returns suggestions only and does not modify data.",
	}, nil
}

func (s *MemoryStore) memStats(projectId string) (map[string]interface{}, error) {
	dbStats := make(map[string]interface{})

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE project_id = ?", projectId).Scan(&count); err == nil {
		dbStats["sessions"] = count
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM events WHERE project_id = ?", projectId).Scan(&count); err == nil {
		dbStats["events"] = count
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM decisions WHERE project_id = ?", projectId).Scan(&count); err == nil {
		dbStats["decisions"] = count
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM reasoning_memories WHERE project_id = ?", projectId).Scan(&count); err == nil {
		dbStats["lessons"] = count
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM memory_graph_nodes WHERE project_id = ?", projectId).Scan(&count); err == nil {
		dbStats["graph_nodes"] = count
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM memory_graph_edges WHERE project_id = ?", projectId).Scan(&count); err == nil {
		dbStats["graph_edges"] = count
	}

	result := map[string]interface{}{
		"project_id": projectId,
		"db":         dbStats,
	}

	if s.rt != nil {
		runtimeStats := map[string]interface{}{
			"startup_cache_size":   s.rt.StartupCache().Len(),
			"retrieval_cache_size": s.rt.RetrievalCache().Len(),
			"hot_lessons":          s.rt.HotLessons().Len(),
			"session_states":       s.rt.SessionState().Len(),
			"snapshots":            len(s.rt.Snapshots()),
			"write_queue_pending":  len(s.rt.WriteQueue()),
		}
		result["runtime"] = runtimeStats
	}

	return result, nil
}

func (s *MemoryStore) listSessions(projectId string, filter string) (map[string]interface{}, error) {
	if filter == "" {
		filter = "all"
	}
	if filter != "active" && filter != "closed" && filter != "all" {
		return nil, &MemoryError{"filter must be 'active', 'closed', or 'all'."}
	}

	var where string
	var args []interface{}
	args = append(args, projectId)
	if filter != "all" {
		where = " AND status = ?"
		args = append(args, filter)
	}

	var activeCount, closedCount int
	s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE project_id = ? AND status = 'active'", projectId).Scan(&activeCount)
	s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE project_id = ? AND status = 'closed'", projectId).Scan(&closedCount)

	sessions, err := s.queryRows(
		"SELECT id, agent_name, namespace, status, summary, started_at, closed_at FROM sessions WHERE project_id = ?"+where+" ORDER BY started_at DESC LIMIT 50",
		args...,
	)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"sessions":      sessions,
		"total":         len(sessions),
		"active_count":  activeCount,
		"closed_count":  closedCount,
		"filter":        filter,
	}, nil
}

func (s *MemoryStore) ensureProject(projectPath string) (map[string]interface{}, error) {
	root, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("resolve project path: %w", err)
	}
	pid := s.projectId
	if pid == "" {
		h := sha256.Sum256([]byte(root))
		pid = hex.EncodeToString(h[:])[:16]
	}

	existing, err := s.queryRow("SELECT * FROM projects WHERE id = ?", pid)
	if err == nil && existing != nil {
		return existing, nil
	}

	if err := s.assertWritable(); err != nil {
		return nil, err
	}

	now := isoNow()
	name := filepath.Base(root)
	_, err = s.db.Exec(
		"INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		pid, name, root, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("ensure project: %w", err)
	}

	s.invalidateProject(pid)
	return map[string]interface{}{
		"id": pid, "name": name, "root_path": root,
		"created_at": now, "updated_at": now,
	}, nil
}

func (s *MemoryStore) projectRootPath(projectId string) string {
	row, _ := s.queryRow("SELECT root_path FROM projects WHERE id = ?", projectId)
	if row == nil {
		return ""
	}
	return strVal(row, "root_path")
}

func (s *MemoryStore) checkpointWAL() {
	s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}

func (s *MemoryStore) requireSession(sessionId string) (map[string]interface{}, error) {
	session, err := s.queryRow("SELECT * FROM sessions WHERE id = ?", sessionId)
	if err != nil || session == nil {
		return nil, &MemoryError{"Cannot find session '" + sessionId + "'. Recommended action: call memory_start_session before writing memory."}
	}
	if session["status"] != "active" {
		return nil, &MemoryError{"Cannot write to session '" + sessionId + "' because status is '" + strVal(session, "status") + "'. Recommended action: start a new session."}
	}
	return session, nil
}

func (s *MemoryStore) insertEvent(projectId string, sessionId string, kind string, content string, evidenceRefs []string) int64 {
	evJSON, _ := json.Marshal(redactList(evidenceRefs))
	result, err := s.db.Exec(
		"INSERT INTO events (project_id, session_id, kind, content, evidence_refs, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		projectId, sessionId, kind, content, string(evJSON), isoNow(),
	)
	if err != nil {
		return 0
	}
	id, _ := result.LastInsertId()
	return id
}

func (s *MemoryStore) assertWritable() error {
	if s.readonly {
		return &MemoryError{"Mem is running in readonly mode. Recommended action: unset MEM_READONLY or use read-only tools only."}
	}
	return nil
}

func (s *MemoryStore) retrievalCacheKey(projectId, query string, tags []string, limit int, format string) string {
	h := sha256.New()
	h.Write([]byte(projectId))
	h.Write([]byte(query))
	for _, t := range tags {
		h.Write([]byte(t))
	}
	h.Write([]byte(fmt.Sprintf("%d", limit)))
	h.Write([]byte(format))
	return projectId + ":" + hex.EncodeToString(h.Sum(nil))[:16]
}

func (s *MemoryStore) queryRow(query string, args ...interface{}) (map[string]interface{}, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanRowToMap(rows)
}

func (s *MemoryStore) queryRows(query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		m, err := scanRowToMap(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}
