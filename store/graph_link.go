package store

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
