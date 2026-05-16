package store

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
