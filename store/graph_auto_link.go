package store

import (
	"fmt"
	"regexp"
	"strings"
)

var pathExtRe = regexp.MustCompile(`[\w.\-/\\]+\.(go|ts|js|tsx|jsx|py|rs|java|rb|php|c|cpp|h|hpp|sql|yaml|yml|json|toml|md|css|html|xml|sh|ps1|txt|mod|sum|vue|svelte|tf|proto|graphql)(\b|$)`)

func (s *MemoryStore) graphAutoLinkFeatureFromNamespace(projectId string, sessionId string, namespace *string) {
	if namespace == nil || *namespace == "" {
		return
	}
	s.graphLink(projectId, "Session", sessionId, &sessionId, "Feature", *namespace, namespace, "WORKED_ON", nil, &sessionId, nil)
}

func (s *MemoryStore) graphAutoLinkFileChanges(projectId string, sessionId string, content string, evidenceRefs []string) {
	files := s.graphExtractFilePaths(content, evidenceRefs)
	if len(files) == 0 {
		return
	}
	featureLabel := s.graphFindFeatureForSession(projectId, sessionId)
	fromType := "Session"
	fromLabel := sessionId
	if featureLabel != "" {
		fromType = "Feature"
		fromLabel = featureLabel
	}
	for _, f := range files {
		s.graphLink(projectId, fromType, fromLabel, nil, "File", f, &f, "TOUCHED", evidenceRefs, &sessionId, nil)
	}
}

func (s *MemoryStore) graphAutoLinkBlocker(projectId string, sessionId string, content string) {
	label := content
	if len(label) > 120 {
		label = label[:120]
	}
	featureLabel := s.graphFindFeatureForSession(projectId, sessionId)
	fromType := "Session"
	fromLabel := sessionId
	if featureLabel != "" {
		fromType = "Feature"
		fromLabel = featureLabel
	}
	s.graphLink(projectId, fromType, fromLabel, nil, "Blocker", label, nil, "BLOCKED_BY", nil, &sessionId, nil)
}

func (s *MemoryStore) graphAutoLinkDecision(projectId string, sessionId string, decisionId int64, decisionText string, evidenceRefs []string) {
	label := decisionText
	if len(label) > 100 {
		label = label[:100]
	}
	ref := fmt.Sprintf("decision:%d", decisionId)
	s.graphLink(projectId, "Session", sessionId, &sessionId, "Decision", label, &ref, "MADE", evidenceRefs, &sessionId, nil)
	featureLabel := s.graphFindFeatureForSession(projectId, sessionId)
	if featureLabel != "" {
		s.graphLink(projectId, "Feature", featureLabel, nil, "Decision", label, &ref, "MADE", evidenceRefs, &sessionId, nil)
	}
}

func (s *MemoryStore) graphAutoLinkLesson(projectId string, lessonId int64, title string, sourceSessionId string, tags []string) {
	ref := fmt.Sprintf("lesson:%d", lessonId)
	featureLabel := s.graphFindFeatureForSession(projectId, sourceSessionId)
	if featureLabel != "" {
		s.graphLink(projectId, "Feature", featureLabel, nil, "Lesson", title, &ref, "DERIVED_FROM", nil, &sourceSessionId, nil)
	}
	s.graphLink(projectId, "Session", sourceSessionId, &sourceSessionId, "Lesson", title, &ref, "PRODUCED", nil, &sourceSessionId, nil)
}

func (s *MemoryStore) graphAutoLinkEvidence(projectId string, sessionId string, eventId int64, content string, evidenceRefs []string) {
	label := content
	if len(label) > 100 {
		label = label[:100]
	}
	ref := fmt.Sprintf("event:%d", eventId)
	s.graphLink(projectId, "Session", sessionId, &sessionId, "Evidence", label, &ref, "PRODUCED", evidenceRefs, &sessionId, &ref)
	featureLabel := s.graphFindFeatureForSession(projectId, sessionId)
	if featureLabel != "" {
		s.graphLink(projectId, "Feature", featureLabel, nil, "Evidence", label, &ref, "PRODUCED", evidenceRefs, &sessionId, &ref)
	}
}

func (s *MemoryStore) graphFindFeatureForSession(projectId string, sessionId string) string {
	edges, _ := s.queryRows(
		"SELECT * FROM memory_graph_edges WHERE project_id = ? AND from_node_id IN (SELECT id FROM memory_graph_nodes WHERE project_id = ? AND type = 'Session' AND label = ?) AND relationship = 'WORKED_ON' LIMIT 1",
		projectId, projectId, sessionId,
	)
	if len(edges) == 0 {
		return ""
	}
	toId := int64(intVal(edges[0], "to_node_id"))
	node, err := s.queryRow("SELECT label FROM memory_graph_nodes WHERE id = ? AND type = 'Feature'", toId)
	if err != nil || node == nil {
		return ""
	}
	return strVal(node, "label")
}

func (s *MemoryStore) graphExtractFilePaths(content string, evidenceRefs []string) []string {
	seen := make(map[string]bool)
	var files []string
	for _, match := range pathExtRe.FindAllString(content, -1) {
		cleaned := s.graphCleanPath(match)
		if cleaned != "" && !seen[cleaned] {
			seen[cleaned] = true
			files = append(files, cleaned)
		}
	}
	for _, ref := range evidenceRefs {
		cleaned := s.graphCleanPath(ref)
		if cleaned != "" && !seen[cleaned] {
			if pathExtRe.MatchString(cleaned) {
				seen[cleaned] = true
				files = append(files, cleaned)
			}
		}
	}
	return files
}

func (s *MemoryStore) graphCleanPath(raw string) string {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.ReplaceAll(cleaned, "\\", "/")
	cleaned = strings.TrimRight(cleaned, "/")
	if cleaned == "" {
		return ""
	}
	return cleaned
}

func (s *MemoryStore) graphBuildStartupContext(projectId string) map[string]interface{} {
	activeFeatures, _ := s.queryRows(
		"SELECT DISTINCT n.label, n.id FROM memory_graph_nodes n JOIN memory_graph_edges e ON e.to_node_id = n.id WHERE e.relationship = 'WORKED_ON' AND e.project_id = ? AND n.type = 'Feature' AND e.from_node_id IN (SELECT id FROM memory_graph_nodes WHERE project_id = ? AND type = 'Session' AND entity_ref IN (SELECT id FROM sessions WHERE project_id = ? AND status = 'active'))",
		projectId, projectId, projectId,
	)
	recentBlockers, _ := s.queryRows(
		"SELECT n.label, n.id, e.created_at FROM memory_graph_edges e JOIN memory_graph_nodes n ON n.id = e.to_node_id WHERE e.project_id = ? AND e.relationship = 'BLOCKED_BY' AND n.type = 'Blocker' ORDER BY e.id DESC LIMIT 5",
		projectId,
	)
	recentFiles, _ := s.queryRows(
		"SELECT DISTINCT n.label, n.id FROM memory_graph_edges e JOIN memory_graph_nodes n ON n.id = e.to_node_id WHERE e.project_id = ? AND e.relationship = 'TOUCHED' AND n.type = 'File' ORDER BY e.id DESC LIMIT 10",
		projectId,
	)

	result := make(map[string]interface{})
	if len(activeFeatures) > 0 {
		result["active_features"] = activeFeatures
	}
	if len(recentBlockers) > 0 {
		result["recent_blockers"] = recentBlockers
	}
	if len(recentFiles) > 0 {
		result["recent_files"] = recentFiles
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (s *MemoryStore) graphUpdateSessionSummary(sessionId string, summary *string) {
	if summary == nil || *summary == "" {
		return
	}
	meta := fmt.Sprintf(`{"summary": %q}`, *summary)
	s.db.Exec("UPDATE memory_graph_nodes SET metadata_json = ? WHERE type = 'Session' AND label = ?", meta, sessionId)
}
