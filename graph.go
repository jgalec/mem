package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var validNodeTypes = map[string]bool{
	"Project": true, "Feature": true, "Session": true, "Decision": true,
	"Evidence": true, "Lesson": true, "Blocker": true, "File": true, "Command": true,
}

var validRelationships = map[string]bool{
	"WORKED_ON": true, "TOUCHED": true, "MADE": true, "SUPPORTED_BY": true,
	"PRODUCED": true, "FROM_COMMAND": true, "DERIVED_FROM": true,
	"BLOCKED_BY": true, "DEPENDS_ON": true,
}

func (s *MemoryStore) graphLink(projectId string, fromType string, fromLabel string, fromEntityRef *string, toType string, toLabel string, toEntityRef *string, relationship string, evidenceRefs []string, sourceSessionId *string, sourceEntity *string) (map[string]interface{}, error) {
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	if err := s.graphValidateProject(projectId); err != nil {
		return nil, err
	}
	if err := s.graphValidateTypes(fromType, toType, relationship); err != nil {
		return nil, err
	}

	fromNode, err := s.graphUpsertNode(projectId, fromType, fromLabel, fromEntityRef)
	if err != nil {
		return nil, err
	}
	toNode, err := s.graphUpsertNode(projectId, toType, toLabel, toEntityRef)
	if err != nil {
		return nil, err
	}

	edge, err := s.graphCreateEdge(projectId, intVal(fromNode, "id"), intVal(toNode, "id"), relationship, evidenceRefs, sourceSessionId, sourceEntity)
	if err != nil {
		return nil, err
	}

	s.invalidateProject(projectId)

	return map[string]interface{}{
		"status": "linked",
		"from":   fromNode,
		"to":     toNode,
		"edge":   edge,
	}, nil
}

func (s *MemoryStore) graphBatchLink(links []map[string]interface{}) (map[string]interface{}, error) {
	if err := s.assertWritable(); err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, &MemoryError{"batch_link requires at least one link."}
	}

	var results []map[string]interface{}
	projectIds := make(map[string]bool)

	for _, link := range links {
		projectId := requireString(link, "project_id")
		fromType := requireString(link, "from_type")
		fromLabel := requireString(link, "from_label")
		toType := requireString(link, "to_type")
		toLabel := requireString(link, "to_label")
		relationship := requireString(link, "relationship")

		if projectId == "" || fromType == "" || fromLabel == "" || toType == "" || toLabel == "" || relationship == "" {
			return nil, &MemoryError{"batch_link: each link requires project_id, from_type, from_label, to_type, to_label, relationship."}
		}

		fromEntityRef := optString(link, "from_entity_ref")
		toEntityRef := optString(link, "to_entity_ref")
		evidenceRefs := getStringSlice(link, "evidence_refs")
		sourceSessionId := optString(link, "source_session_id")
		sourceEntity := optString(link, "source_entity")

		result, err := s.graphLink(projectId, fromType, fromLabel, fromEntityRef, toType, toLabel, toEntityRef, relationship, evidenceRefs, sourceSessionId, sourceEntity)
		if err != nil {
			return nil, &MemoryError{"batch_link failed at link " + fromLabel + " -> " + toLabel + ": " + err.Error()}
		}
		results = append(results, result)
		projectIds[projectId] = true
	}

	for pid := range projectIds {
		s.invalidateProject(pid)
	}

	return map[string]interface{}{
		"links":  results,
		"count":  len(results),
	}, nil
}

func (s *MemoryStore) graphNeighbors(nodeId int64, direction string, relationship *string) (map[string]interface{}, error) {
	if direction == "" {
		direction = "both"
	}
	if direction != "outgoing" && direction != "incoming" && direction != "both" {
		return nil, &MemoryError{"direction must be outgoing, incoming, or both."}
	}

	node, err := s.graphGetNode(nodeId)
	if err != nil {
		return nil, err
	}

	var edges []map[string]interface{}
	var neighborIds []int64
	if direction == "outgoing" || direction == "both" {
		e, ids := s.graphQueryEdges(nodeId, "from_node_id", "to_node_id", relationship)
		edges = append(edges, e...)
		neighborIds = append(neighborIds, ids...)
	}
	if direction == "incoming" || direction == "both" {
		e, ids := s.graphQueryEdges(nodeId, "to_node_id", "from_node_id", relationship)
		edges = append(edges, e...)
		neighborIds = append(neighborIds, ids...)
	}

	neighbors := s.graphFetchNodes(neighborIds)

	return map[string]interface{}{
		"node":      node,
		"direction": direction,
		"neighbors": neighbors,
		"edges":     edges,
	}, nil
}

func (s *MemoryStore) graphTraceFeature(projectId string, featureLabel *string, nodeId *int64) (map[string]interface{}, error) {
	node, err := s.graphResolveNode(projectId, "Feature", featureLabel, nodeId)
	if err != nil {
		return nil, err
	}

	neighbors, err := s.graphNeighbors(int64(intVal(node, "id")), "both", nil)
	if err != nil {
		return nil, err
	}
	edges := s.graphGetEdgesAsMaps(neighbors["edges"])

	decisions := s.graphFilterNodesByType(neighbors["neighbors"], "Decision", edges)
	evidence := s.graphFilterNodesByType(neighbors["neighbors"], "Evidence", edges)
	files := s.graphFilterNodesByType(neighbors["neighbors"], "File", edges)
	blockers := s.graphFilterNodesByType(neighbors["neighbors"], "Blocker", edges)
	dependencies := s.graphFilterNodesByType(neighbors["neighbors"], "Feature", edges)
	sessions := s.graphFilterNodesByType(neighbors["neighbors"], "Session", edges)

	return map[string]interface{}{
		"feature":      node,
		"decisions":    s.dedupeNodes(decisions),
		"evidence":     s.dedupeNodes(evidence),
		"files":        s.dedupeNodes(files),
		"blockers":     s.dedupeNodes(blockers),
		"dependencies": s.filterOutNode(s.dedupeNodes(dependencies), intVal(node, "id")),
		"sessions":     s.dedupeNodes(sessions),
	}, nil
}

func (s *MemoryStore) graphTraceFile(projectId string, filePath *string, nodeId *int64) (map[string]interface{}, error) {
	var node map[string]interface{}
	var err error

	if filePath != nil && *filePath != "" {
		node, err = s.graphFindFileNode(projectId, *filePath)
		if err != nil {
			return nil, err
		}
	} else if nodeId != nil {
		node, err = s.graphGetNode(*nodeId)
		if err != nil || strVal(node, "type") != "File" {
			return nil, &MemoryError{"Node is not a File. Provide a file_path or a File node_id."}
		}
	} else {
		return nil, &MemoryError{"Provide file_path or node_id to trace a file."}
	}

	neighbors, err := s.graphNeighbors(int64(intVal(node, "id")), "both", nil)
	if err != nil {
		return nil, err
	}
	edges := s.graphGetEdgesAsMaps(neighbors["edges"])

	features := s.graphFilterNodesByType(neighbors["neighbors"], "Feature", edges)
	evidence := s.graphFilterNodesByType(neighbors["neighbors"], "Evidence", edges)
	lessons := s.graphFilterNodesByType(neighbors["neighbors"], "Lesson", edges)
	commands := s.graphFilterNodesByType(neighbors["neighbors"], "Command", edges)

	return map[string]interface{}{
		"file":     node,
		"features": s.dedupeNodes(features),
		"evidence": s.dedupeNodes(evidence),
		"lessons":  s.dedupeNodes(lessons),
		"commands": s.dedupeNodes(commands),
	}, nil
}

func (s *MemoryStore) graphFindRelatedLessons(projectId string, featureLabel *string, filePath *string, nodeId *int64) (map[string]interface{}, error) {
	var node map[string]interface{}
	var err error
	var nodeType string

	if nodeId != nil {
		node, err = s.graphGetNode(*nodeId)
		if err != nil {
			return nil, err
		}
		nodeType = strVal(node, "type")
	} else if featureLabel != nil && *featureLabel != "" {
		node, err = s.graphResolveNode(projectId, "Feature", featureLabel, nil)
		if err != nil {
			return nil, err
		}
		nodeType = "Feature"
	} else if filePath != nil && *filePath != "" {
		node, err = s.graphFindFileNode(projectId, *filePath)
		if err != nil {
			return nil, err
		}
		nodeType = "File"
	} else {
		return nil, &MemoryError{"Provide node_id, feature_label, or file_path to find related lessons."}
	}

	nid := intVal(node, "id")
	lessons := s.graphDirectLessons(nid)
	if nodeType == "Feature" {
		files := s.graphConnectedType(nid, "File")
		for _, f := range files {
			fLessons := s.graphDirectLessons(intVal(f, "id"))
			lessons = s.mergeLessonNodes(lessons, fLessons)
		}
	}

	return map[string]interface{}{
		"source":  node,
		"lessons": s.dedupeNodes(lessons),
	}, nil
}

func (s *MemoryStore) graphValidateProject(projectId string) error {
	project, err := s.queryRow("SELECT id FROM projects WHERE id = ?", projectId)
	if err != nil || project == nil {
		return &MemoryError{"Project '" + projectId + "' does not exist. Recommended action: call memory_get_startup_context or memory_start_session first."}
	}
	return nil
}

func (s *MemoryStore) graphValidateTypes(fromType string, toType string, relationship string) error {
	if !validNodeTypes[fromType] {
		return &MemoryError{"Invalid from_type '" + fromType + "'. Valid types: " + graphTypeList()}
	}
	if !validNodeTypes[toType] {
		return &MemoryError{"Invalid to_type '" + toType + "'. Valid types: " + graphTypeList()}
	}
	if !validRelationships[relationship] {
		return &MemoryError{"Invalid relationship '" + relationship + "'. Valid relationships: " + graphRelList()}
	}
	return nil
}

func (s *MemoryStore) graphUpsertNode(projectId string, nodeType string, label string, entityRef *string) (map[string]interface{}, error) {
	if entityRef != nil && *entityRef != "" {
		existing, err := s.queryRow(
			"SELECT * FROM memory_graph_nodes WHERE project_id = ? AND type = ? AND entity_ref = ?",
			projectId, nodeType, *entityRef,
		)
		if err == nil && existing != nil {
			if strVal(existing, "label") != label {
				s.db.Exec("UPDATE memory_graph_nodes SET label = ? WHERE id = ?", label, existing["id"])
				existing["label"] = label
			}
			return hydrateJsonColumns(existing), nil
		}
	}

	existing, err := s.queryRow(
		"SELECT * FROM memory_graph_nodes WHERE project_id = ? AND type = ? AND label = ?",
		projectId, nodeType, label,
	)
	if err == nil && existing != nil {
		if entityRef != nil && *entityRef != "" && existing["entity_ref"] == nil {
			s.db.Exec("UPDATE memory_graph_nodes SET entity_ref = ? WHERE id = ?", *entityRef, existing["id"])
			existing["entity_ref"] = *entityRef
		}
		return hydrateJsonColumns(existing), nil
	}

	now := isoNow()
	var er interface{}
	if entityRef != nil {
		er = *entityRef
	}
	result, err := s.db.Exec(
		"INSERT INTO memory_graph_nodes (project_id, type, entity_ref, label, metadata_json, created_at) VALUES (?, ?, ?, ?, '{}', ?)",
		projectId, nodeType, er, label, now,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert node: %w", err)
	}
	id, _ := result.LastInsertId()
	node, _ := s.queryRow("SELECT * FROM memory_graph_nodes WHERE id = ?", id)
	return hydrateJsonColumns(node), nil
}

func (s *MemoryStore) graphCreateEdge(projectId string, fromId int, toId int, relationship string, evidenceRefs []string, sourceSessionId *string, sourceEntity *string) (map[string]interface{}, error) {
	now := isoNow()
	evJSON, _ := json.Marshal(redactList(evidenceRefs))
	var ssid interface{}
	if sourceSessionId != nil {
		ssid = *sourceSessionId
	}
	var se interface{}
	if sourceEntity != nil {
		se = *sourceEntity
	}

	result, err := s.db.Exec(
		"INSERT INTO memory_graph_edges (project_id, from_node_id, to_node_id, relationship, evidence_refs, source_session_id, source_entity, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		projectId, fromId, toId, relationship, string(evJSON), ssid, se, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create edge: %w", err)
	}
	id, _ := result.LastInsertId()
	edge, _ := s.queryRow("SELECT * FROM memory_graph_edges WHERE id = ?", id)
	return hydrateJsonColumns(edge), nil
}

func (s *MemoryStore) graphGetNode(nodeId int64) (map[string]interface{}, error) {
	node, err := s.queryRow("SELECT * FROM memory_graph_nodes WHERE id = ?", nodeId)
	if err != nil || node == nil {
		return nil, &MemoryError{"Node not found with id " + fmt.Sprintf("%d", nodeId) + ". Link nodes first before querying them."}
	}
	return hydrateJsonColumns(node), nil
}

func (s *MemoryStore) graphResolveNode(projectId string, nodeType string, label *string, nodeId *int64) (map[string]interface{}, error) {
	if nodeId != nil {
		node, err := s.graphGetNode(*nodeId)
		if err != nil {
			return nil, err
		}
		if strVal(node, "type") != nodeType {
			return nil, &MemoryError{"Node is type '" + strVal(node, "type") + "', expected '" + nodeType + "'."}
		}
		return node, nil
	}
	if label != nil && *label != "" {
		node, err := s.queryRow(
			"SELECT * FROM memory_graph_nodes WHERE project_id = ? AND type = ? AND label = ?",
			projectId, nodeType, *label,
		)
		if err != nil || node == nil {
			return nil, &MemoryError{"No " + nodeType + " node found with label '" + *label + "'. Link it first."}
		}
		return hydrateJsonColumns(node), nil
	}
	return nil, &MemoryError{"Provide node_id or label to resolve a " + nodeType + " node."}
}

func (s *MemoryStore) graphFindFileNode(projectId string, filePath string) (map[string]interface{}, error) {
	cleaned := strings.TrimSpace(filePath)
	cleaned = strings.ReplaceAll(cleaned, "\\", "/")
	cleaned = strings.TrimRight(cleaned, "/")

	node, err := s.queryRow(
		"SELECT * FROM memory_graph_nodes WHERE project_id = ? AND type = 'File' AND label = ?",
		projectId, cleaned,
	)
	if err != nil || node == nil {
		if filePath != cleaned {
			node, err = s.queryRow(
				"SELECT * FROM memory_graph_nodes WHERE project_id = ? AND type = 'File' AND label = ?",
				projectId, filePath,
			)
		}
	}
	if err != nil || node == nil {
		return nil, &MemoryError{"File node not found for '" + filePath + "'. Link the file to a feature first."}
	}
	return hydrateJsonColumns(node), nil
}

func (s *MemoryStore) graphQueryEdges(nodeId int64, fromCol string, toCol string, relationship *string) ([]map[string]interface{}, []int64) {
	query := "SELECT * FROM memory_graph_edges WHERE " + fromCol + " = ?"
	var args []interface{}
	args = append(args, nodeId)
	if relationship != nil && *relationship != "" {
		query += " AND relationship = ?"
		args = append(args, *relationship)
	}
	rows, err := s.queryRows(query, args...)
	if err != nil {
		return nil, nil
	}
	var edges []map[string]interface{}
	var ids []int64
	for _, row := range rows {
		edges = append(edges, hydrateJsonColumns(row))
		ids = append(ids, int64(intVal(row, toCol)))
	}
	return edges, ids
}

func (s *MemoryStore) graphFetchNodes(ids []int64) []map[string]interface{} {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := "SELECT * FROM memory_graph_nodes WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	rows, _ := s.queryRows(query, args...)
	var nodes []map[string]interface{}
	for _, row := range rows {
		nodes = append(nodes, hydrateJsonColumns(row))
	}
	return nodes
}

func (s *MemoryStore) graphGetEdgesAsMaps(raw interface{}) []map[string]interface{} {
	if raw == nil {
		return nil
	}
	if arr, ok := raw.([]map[string]interface{}); ok {
		return arr
	}
	if arr, ok := raw.([]interface{}); ok {
		result := make([]map[string]interface{}, len(arr))
		for i, v := range arr {
			if m, ok := v.(map[string]interface{}); ok {
				result[i] = m
			}
		}
		return result
	}
	return nil
}

func (s *MemoryStore) graphFilterNodesByType(rawNeighbors interface{}, nodeType string, edges []map[string]interface{}) []map[string]interface{} {
	var neighbors []map[string]interface{}
	switch v := rawNeighbors.(type) {
	case []map[string]interface{}:
		neighbors = v
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				neighbors = append(neighbors, m)
			}
		}
	default:
		return nil
	}

	edgeMap := make(map[int64]map[string]interface{})
	for _, e := range edges {
		if e != nil {
			edgeMap[int64(intVal(e, "to_node_id"))] = e
			edgeMap[int64(intVal(e, "from_node_id"))] = e
		}
	}

	var result []map[string]interface{}
	for _, n := range neighbors {
		if strVal(n, "type") == nodeType {
			ncopy := make(map[string]interface{})
			for k, v := range n {
				ncopy[k] = v
			}
			if relEdge, ok := edgeMap[int64(intVal(ncopy, "id"))]; ok {
				ncopy["_edge_relationship"] = relEdge["relationship"]
				ncopy["_edge_evidence_refs"] = relEdge["evidence_refs"]
			}
			result = append(result, ncopy)
		}
	}
	return result
}

func (s *MemoryStore) dedupeNodes(nodes []map[string]interface{}) []map[string]interface{} {
	seen := make(map[int64]bool)
	var result []map[string]interface{}
	for _, n := range nodes {
		id := int64(intVal(n, "id"))
		if !seen[id] {
			seen[id] = true
			result = append(result, n)
		}
	}
	return result
}

func (s *MemoryStore) filterOutNode(nodes []map[string]interface{}, excludeId int) []map[string]interface{} {
	var result []map[string]interface{}
	for _, n := range nodes {
		if intVal(n, "id") != excludeId {
			result = append(result, n)
		}
	}
	return result
}

func (s *MemoryStore) graphDirectLessons(nodeId int) []map[string]interface{} {
	edges, _ := s.queryRows(
		"SELECT * FROM memory_graph_edges WHERE (from_node_id = ? OR to_node_id = ?) AND relationship = 'DERIVED_FROM'",
		nodeId, nodeId,
	)
	var lessonIds []int64
	for _, e := range edges {
		if strVal(e, "relationship") == "DERIVED_FROM" {
			if intVal(e, "to_node_id") == nodeId {
				lessonIds = append(lessonIds, int64(intVal(e, "from_node_id")))
			}
		}
	}
	if len(lessonIds) == 0 {
		return nil
	}
	lessons := s.graphFetchNodes(lessonIds)
	var result []map[string]interface{}
	for _, l := range lessons {
		if strVal(l, "type") == "Lesson" {
			result = append(result, l)
		}
	}
	return result
}

func (s *MemoryStore) graphConnectedType(nodeId int, nodeType string) []map[string]interface{} {
	edges, _ := s.queryRows("SELECT * FROM memory_graph_edges WHERE from_node_id = ? OR to_node_id = ?", nodeId, nodeId)
	var neighborIds []int64
	for _, e := range edges {
		if intVal(e, "from_node_id") == nodeId {
			neighborIds = append(neighborIds, int64(intVal(e, "to_node_id")))
		} else {
			neighborIds = append(neighborIds, int64(intVal(e, "from_node_id")))
		}
	}
	nodes := s.graphFetchNodes(neighborIds)
	var result []map[string]interface{}
	for _, n := range nodes {
		if strVal(n, "type") == nodeType {
			result = append(result, n)
		}
	}
	return result
}

func (s *MemoryStore) mergeLessonNodes(existing []map[string]interface{}, additional []map[string]interface{}) []map[string]interface{} {
	seen := make(map[int64]bool)
	for _, l := range existing {
		seen[int64(intVal(l, "id"))] = true
	}
	for _, l := range additional {
		if !seen[int64(intVal(l, "id"))] {
			existing = append(existing, l)
			seen[int64(intVal(l, "id"))] = true
		}
	}
	return existing
}

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

func (s *MemoryStore) graphUpdateSessionSummary(sessionId string, summary *string) {
	if summary == nil || *summary == "" {
		return
	}
	meta := fmt.Sprintf(`{"summary": %q}`, *summary)
	s.db.Exec("UPDATE memory_graph_nodes SET metadata_json = ? WHERE type = 'Session' AND label = ?", meta, sessionId)
}

func graphTypeList() string {
	return "Project, Feature, Session, Decision, Evidence, Lesson, Blocker, File, Command"
}

func graphRelList() string {
	return "WORKED_ON, TOUCHED, MADE, SUPPORTED_BY, PRODUCED, FROM_COMMAND, DERIVED_FROM, BLOCKED_BY, DEPENDS_ON"
}
