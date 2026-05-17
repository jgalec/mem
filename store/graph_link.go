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
