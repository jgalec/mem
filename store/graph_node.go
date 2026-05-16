package store

import (
	"fmt"
	"strings"
)

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
