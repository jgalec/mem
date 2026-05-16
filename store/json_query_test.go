package store

import (
	"encoding/json"
	"testing"
)

func TestJsonQueryFiltersByKind(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	sess, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := sess["session"].(map[string]interface{})["id"].(string)

	store.logEvent(sid, "file_changed", "Created foo.go", nil)
	store.logEvent(sid, "test_run", "Ran tests", nil)
	store.logEvent(sid, "file_changed", "Created bar.go", nil)
	store.logEvent(sid, "blocked", "Missing dep", nil)

	criteria := json.RawMessage(`{"entity":"events","filters":[{"field":"kind","op":"eq","value":"file_changed"}],"limit":10}`)
	result, err := store.jsonQuery(sess["session"].(map[string]interface{})["project_id"].(string), criteria)
	if err != nil {
		t.Fatalf("jsonQuery: %v", err)
	}
	results := result["results"].([]map[string]interface{})
	if len(results) != 2 {
		t.Fatalf("expected 2 file_changed events, got %d", len(results))
	}
	for _, r := range results {
		if r["kind"] != "file_changed" {
			t.Errorf("expected kind=file_changed, got %v", r["kind"])
		}
	}
}

func TestJsonQueryContainsOperator(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	sess, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := sess["session"].(map[string]interface{})["id"].(string)

	store.logEvent(sid, "note", "alpha widget broken", nil)
	store.logEvent(sid, "note", "beta service online", nil)
	store.logEvent(sid, "note", "gamma broken pipe", nil)

	criteria := json.RawMessage(`{"entity":"events","filters":[{"field":"content","op":"contains","value":"broken"}],"limit":10}`)
	result, err := store.jsonQuery(sess["session"].(map[string]interface{})["project_id"].(string), criteria)
	if err != nil {
		t.Fatalf("jsonQuery: %v", err)
	}
	results := result["results"].([]map[string]interface{})
	if len(results) != 2 {
		t.Fatalf("expected 2 events containing 'broken', got %d", len(results))
	}
}

func TestJsonQueryOrCombinator(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	sess, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := sess["session"].(map[string]interface{})["id"].(string)

	store.logEvent(sid, "file_changed", "A", nil)
	store.logEvent(sid, "test_run", "B", nil)
	store.logEvent(sid, "blocked", "C", nil)
	store.logEvent(sid, "progress", "D", nil)

	criteria := json.RawMessage(`{"entity":"events","filters":[{"or":[{"field":"kind","op":"eq","value":"file_changed"},{"field":"kind","op":"eq","value":"blocked"}]}],"limit":10}`)
	result, err := store.jsonQuery(sess["session"].(map[string]interface{})["project_id"].(string), criteria)
	if err != nil {
		t.Fatalf("jsonQuery: %v", err)
	}
	results := result["results"].([]map[string]interface{})
	if len(results) != 2 {
		t.Fatalf("expected 2 events (file_changed OR blocked), got %d", len(results))
	}
}

func TestJsonQueryNotFilter(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	sess, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := sess["session"].(map[string]interface{})["id"].(string)

	store.logEvent(sid, "file_changed", "X", nil)
	store.logEvent(sid, "file_changed", "Y", nil)
	store.logEvent(sid, "note", "Z", nil)

	criteria := json.RawMessage(`{"entity":"events","filters":[{"not":{"field":"kind","op":"eq","value":"note"}}],"limit":10}`)
	result, err := store.jsonQuery(sess["session"].(map[string]interface{})["project_id"].(string), criteria)
	if err != nil {
		t.Fatalf("jsonQuery: %v", err)
	}
	results := result["results"].([]map[string]interface{})
	if len(results) != 2 {
		t.Fatalf("expected 2 non-note events, got %d", len(results))
	}
}

func TestJsonQueryBetweenOperator(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	sess, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := sess["session"].(map[string]interface{})["id"].(string)

	store.logDecision(sid, "d1", nil, nil, nil, "low")
	store.logDecision(sid, "d2", nil, nil, nil, "medium")
	store.logDecision(sid, "d3", nil, nil, nil, "high")

	criteria := json.RawMessage(`{"entity":"decisions","filters":[{"field":"id","op":"between","value":[2,3]}],"limit":10}`)
	result, err := store.jsonQuery(sess["session"].(map[string]interface{})["project_id"].(string), criteria)
	if err != nil {
		t.Fatalf("jsonQuery: %v", err)
	}
	results := result["results"].([]map[string]interface{})
	if len(results) != 2 {
		t.Fatalf("expected 2 decisions with id between 2 and 3, got %d", len(results))
	}
}

func TestJsonQueryIsNull(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	sess, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := sess["session"].(map[string]interface{})["id"].(string)

	store.logDecision(sid, "decision A", ptr("test rationale"), nil, nil, "medium")
	store.logDecision(sid, "decision B", nil, nil, nil, "low")

	criteria := json.RawMessage(`{"entity":"decisions","filters":[{"field":"rationale","op":"not_null"}],"limit":10}`)
	result, err := store.jsonQuery(sess["session"].(map[string]interface{})["project_id"].(string), criteria)
	if err != nil {
		t.Fatalf("jsonQuery: %v", err)
	}
	results := result["results"].([]map[string]interface{})
	if len(results) != 1 {
		t.Fatalf("expected 1 decision with rationale, got %d", len(results))
	}
	if results[0]["decision"] != "decision A" {
		t.Errorf("expected 'decision A', got %v", results[0]["decision"])
	}
}

func TestJsonQueryPagination(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	sess, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := sess["session"].(map[string]interface{})["id"].(string)

	for i := 0; i < 5; i++ {
		store.logEvent(sid, "note", "event", nil)
	}

	criteria := json.RawMessage(`{"entity":"events","filters":[],"limit":2,"offset":2}`)
	result, err := store.jsonQuery(sess["session"].(map[string]interface{})["project_id"].(string), criteria)
	if err != nil {
		t.Fatalf("jsonQuery: %v", err)
	}
	results := result["results"].([]map[string]interface{})
	if len(results) != 2 {
		t.Fatalf("expected 2 results with limit=2, got %d", len(results))
	}
	offsetVal := result["offset"]
	if offsetVal.(int) != 2 {
		t.Errorf("expected offset=2, got %v", offsetVal)
	}
}

func TestJsonQueryLessonsByStatus(t *testing.T) {
	store := newFixture(t)
	dir := t.TempDir()

	sess, err := store.startSession(dir, ptr("test"), nil, false)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	sid := sess["session"].(map[string]interface{})["id"].(string)
	pid := sess["session"].(map[string]interface{})["project_id"].(string)

	store.addLesson(pid, "L1", "desc1", "content1", sid, nil, "observed", nil, nil)
	store.addLesson(pid, "L2", "desc2", "content2", sid, nil, "consolidated", nil, nil)
	store.addLesson(pid, "L3", "desc3", "content3", sid, nil, "observed", nil, nil)

	criteria := json.RawMessage(`{"entity":"lessons","filters":[{"field":"status","op":"eq","value":"consolidated"}],"limit":10}`)
	result, err := store.jsonQuery(pid, criteria)
	if err != nil {
		t.Fatalf("jsonQuery: %v", err)
	}
	results := result["results"].([]map[string]interface{})
	if len(results) != 1 {
		t.Fatalf("expected 1 consolidated lesson, got %d", len(results))
	}
	if results[0]["title"] != "L2" {
		t.Errorf("expected title L2, got %v", results[0]["title"])
	}
}

func TestJsonQueryUnknownEntity(t *testing.T) {
	store := newFixture(t)

	criteria := json.RawMessage(`{"entity":"unknown","filters":[],"limit":10}`)
	_, err := store.jsonQuery("proj123", criteria)
	if err == nil {
		t.Fatal("expected error for unknown entity")
	}
}

func TestJsonQueryDisallowedColumn(t *testing.T) {
	store := newFixture(t)

	criteria := json.RawMessage(`{"entity":"events","filters":[{"field":"password","op":"eq","value":"secret"}],"limit":10}`)
	_, err := store.jsonQuery("proj123", criteria)
	if err == nil {
		t.Fatal("expected error for disallowed column")
	}
}
