package store

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type QueryCriteria struct {
	Entity  string        `json:"entity"`
	Filters []FilterClause `json:"filters"`
	OrderBy string        `json:"order_by"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
}

type FilterClause struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
	Or    []FilterClause `json:"or,omitempty"`
	Not   *FilterClause  `json:"not,omitempty"`
}

var allowedEntities = map[string][]string{
	"events": {
		"id", "project_id", "session_id", "kind", "content", "created_at",
	},
	"decisions": {
		"id", "project_id", "session_id", "decision", "rationale", "confidence", "created_at",
	},
	"lessons": {
		"id", "project_id", "title", "description", "content", "source_session_id",
		"source_outcome", "status", "confidence", "occurrences", "failure_mode",
		"created_at", "last_reinforced_at", "last_used_at",
	},
	"sessions": {
		"id", "project_id", "agent_name", "namespace", "status", "summary",
		"started_at", "closed_at",
	},
}

var allowedOps = map[string]bool{
	"eq": true, "neq": true, "contains": true, "gt": true, "gte": true,
	"lt": true, "lte": true, "between": true, "in": true,
	"is_null": true, "not_null": true,
}

var entityTables = map[string]string{
	"events":    "events",
	"decisions": "decisions",
	"lessons":   "reasoning_memories",
	"sessions":  "sessions",
}

var allowedOrderBy = regexp.MustCompile(`(?i)^[a-z_]+(\s+(ASC|DESC))?$`)

func (s *MemoryStore) jsonQuery(projectId string, rawCriteria json.RawMessage) (map[string]interface{}, error) {
	var qc QueryCriteria
	if err := json.Unmarshal(rawCriteria, &qc); err != nil {
		return nil, &MemoryError{"Invalid query criteria: " + err.Error()}
	}

	table, ok := entityTables[qc.Entity]
	if !ok {
		return nil, &MemoryError{"Unknown entity '" + qc.Entity + "'. Use: events, decisions, lessons, or sessions."}
	}

	cols, ok := allowedEntities[qc.Entity]
	if !ok {
		return nil, &MemoryError{"Unknown entity '" + qc.Entity + "'."}
	}

	var conditions []string
	var args []interface{}
	argIdx := 1

	if !hasProjectColumn(qc.Entity) {
		conditions = append(conditions, "project_id = ?")
		args = append(args, projectId)
	}

	for _, filter := range qc.Filters {
		cond, condArgs, err := buildFilter(qc.Entity, cols, filter, &argIdx)
		if err != nil {
			return nil, err
		}
		if cond != "" {
			conditions = append(conditions, cond)
			args = append(args, condArgs...)
		}
	}

	if qc.Limit <= 0 {
		qc.Limit = 50
	}
	if qc.Limit > 200 {
		qc.Limit = 200
	}

	orderClause := "ORDER BY id DESC"
	if qc.OrderBy != "" {
		if !allowedOrderBy.MatchString(qc.OrderBy) {
			return nil, &MemoryError{"Invalid order_by: " + qc.OrderBy}
		}
		parts := strings.Fields(strings.ToUpper(qc.OrderBy))
		for i, p := range parts {
			parts[i] = strings.ToLower(p)
		}
		orderClause = "ORDER BY " + qc.OrderBy
	}

	query := "SELECT * FROM " + table
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " " + orderClause + " LIMIT ? OFFSET ?"
	args = append(args, qc.Limit, qc.Offset)

	rows, err := s.queryRows(query, args...)
	if err != nil {
		return nil, &MemoryError{"Query execution failed: " + err.Error()}
	}

	hydrated := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		hydrated[i] = hydrateJsonColumns(row)
	}

	return map[string]interface{}{
		"entity":  qc.Entity,
		"results": hydrated,
		"count":   len(hydrated),
		"limit":   qc.Limit,
		"offset":  qc.Offset,
	}, nil
}

func buildFilter(entity string, cols []string, filter FilterClause, argIdx *int) (string, []interface{}, error) {
	if filter.Not != nil {
		inner, innerArgs, err := buildFilter(entity, cols, *filter.Not, argIdx)
		if err != nil {
			return "", nil, err
		}
		return "NOT (" + inner + ")", innerArgs, nil
	}

	if len(filter.Or) > 0 {
		var orParts []string
		var orArgs []interface{}
		for _, sub := range filter.Or {
			subCond, subArgs, err := buildFilter(entity, cols, sub, argIdx)
			if err != nil {
				return "", nil, err
			}
			if subCond != "" {
				orParts = append(orParts, subCond)
				orArgs = append(orArgs, subArgs...)
			}
		}
		if len(orParts) == 0 {
			return "", nil, nil
		}
		return "(" + strings.Join(orParts, " OR ") + ")", orArgs, nil
	}

	if filter.Field == "" || filter.Op == "" {
		return "", nil, nil
	}

	if !isColumnAllowed(cols, filter.Field) {
		return "", nil, &MemoryError{"Field '" + filter.Field + "' is not queryable on entity '" + entity + "'"}
	}

	if !allowedOps[filter.Op] {
		return "", nil, &MemoryError{"Unsupported operator: " + filter.Op}
	}

	switch filter.Op {
	case "is_null":
		return filter.Field + " IS NULL", nil, nil
	case "not_null":
		return filter.Field + " IS NOT NULL", nil, nil
	case "between":
		arr, ok := filter.Value.([]interface{})
		if !ok || len(arr) != 2 {
			return "", nil, &MemoryError{"'between' requires an array of exactly 2 values"}
		}
		*argIdx += 2
		return filter.Field + " BETWEEN ? AND ?", []interface{}{arr[0], arr[1]}, nil
	case "in":
		arr, ok := filter.Value.([]interface{})
		if !ok || len(arr) == 0 {
			return "", nil, &MemoryError{"'in' requires a non-empty array of values"}
		}
		placeholders := make([]string, len(arr))
		vals := make([]interface{}, len(arr))
		for i, v := range arr {
			placeholders[i] = "?"
			vals[i] = v
		}
		*argIdx += len(arr)
		return filter.Field + " IN (" + strings.Join(placeholders, ", ") + ")", vals, nil
	case "contains":
		*argIdx++
		return filter.Field + " LIKE ?", []interface{}{"%" + fmt.Sprint(filter.Value) + "%"}, nil
	case "gt":
		*argIdx++
		return filter.Field + " > ?", []interface{}{filter.Value}, nil
	case "gte":
		*argIdx++
		return filter.Field + " >= ?", []interface{}{filter.Value}, nil
	case "lt":
		*argIdx++
		return filter.Field + " < ?", []interface{}{filter.Value}, nil
	case "lte":
		*argIdx++
		return filter.Field + " <= ?", []interface{}{filter.Value}, nil
	case "neq":
		*argIdx++
		return filter.Field + " != ?", []interface{}{filter.Value}, nil
	default:
		*argIdx++
		return filter.Field + " = ?", []interface{}{filter.Value}, nil
	}
}

func isColumnAllowed(cols []string, field string) bool {
	for _, c := range cols {
		if c == field {
			return true
		}
	}
	return false
}

func hasProjectColumn(entity string) bool {
	return entity == "lessons" || entity == "decisions" || entity == "events"
}


