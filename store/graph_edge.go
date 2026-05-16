package store

import (
	"encoding/json"
	"fmt"
)

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
