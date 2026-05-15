package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTools(server *mcp.Server, store *MemoryStore) {
	h := toolHandler

	server.AddTool(&mcp.Tool{
		Name:        "memory_get_startup_context",
		Description: "Get compact project memory: active sessions, recent decisions, relevant lessons, and optional recent events.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project_path":{"type":"string"},"response_format":{"type":"string","enum":["concise","detailed"]}},"required":["project_path"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.getStartupContext(requireString(args, "project_path"), optStringDefault(args, "response_format", "concise"))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_start_session",
		Description: "Create a memory session for a project. Pass continue_existing=true only when intentionally resuming the latest active session.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project_path":{"type":"string"},"agent_name":{"type":"string"},"namespace":{"type":"string"},"continue_existing":{"type":"boolean","default":false}},"required":["project_path"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.startSession(requireString(args, "project_path"), optString(args, "agent_name"), optString(args, "namespace"), optBool(args, "continue_existing", false))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_close_session",
		Description: "Close an active memory session. Optionally set a summary of what was remembered or accomplished.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"summary":{"type":"string"}},"required":["session_id"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.closeSession(requireString(args, "session_id"), optString(args, "summary"))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_log_event",
		Description: "Record a relevant memory event in the current session. This does not control work state.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"kind":{"type":"string","enum":["note","progress","tool_run","file_changed","test_run","docs_checked","blocked","closed"],"default":"note"},"content":{"type":"string"},"evidence_refs":{"type":"array","items":{"type":"string"},"default":[]}},"required":["session_id","content"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.logEvent(requireString(args, "session_id"), optStringDefault(args, "kind", "note"), requireString(args, "content"), getStringSlice(args, "evidence_refs"))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_log_decision",
		Description: "Record a technical or project decision. Rationale and evidence_refs are stored separately.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"decision":{"type":"string"},"rationale":{"type":"string"},"alternatives_considered":{"type":"array","items":{"type":"string"},"default":[]},"evidence_refs":{"type":"array","items":{"type":"string"},"default":[]},"confidence":{"type":"string","enum":["low","medium","high"],"default":"medium"}},"required":["session_id","decision"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.logDecision(requireString(args, "session_id"), requireString(args, "decision"), optString(args, "rationale"), getStringSlice(args, "alternatives_considered"), getStringSlice(args, "evidence_refs"), optStringDefault(args, "confidence", "medium"))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_search_lessons",
		Description: "Search strategic lessons with precision-oriented keyword and tag matching.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project_id":{"type":"string"},"query":{"type":"string"},"tags":{"type":"array","items":{"type":"string"},"default":[]},"limit":{"type":"number","default":5},"response_format":{"type":"string","enum":["concise","detailed"]}},"required":["project_id"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.searchLessons(requireString(args, "project_id"), optStringDefault(args, "query", ""), getStringSlice(args, "tags"), optInt(args, "limit", 5), optStringDefault(args, "response_format", "concise"))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_add_lesson",
		Description: "Store a strategic lesson tied to a source memory session.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project_id":{"type":"string"},"title":{"type":"string"},"description":{"type":"string"},"content":{"type":"string"},"source_session_id":{"type":"string"},"source_outcome":{"type":"string","enum":["success","failure"]},"status":{"type":"string","enum":["observed","hypothesis","consolidated"],"default":"observed"},"tags":{"type":"array","items":{"type":"string"},"default":[]},"evidence_refs":{"type":"array","items":{"type":"string"},"default":[]}},"required":["project_id","title","description","content","source_session_id"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.addLesson(requireString(args, "project_id"), requireString(args, "title"), requireString(args, "description"), requireString(args, "content"), requireString(args, "source_session_id"), optString(args, "source_outcome"), optStringDefault(args, "status", "observed"), getStringSlice(args, "tags"), getStringSlice(args, "evidence_refs"))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_reinforce_lesson",
		Description: "Reinforce an existing lesson when it proves useful again, without creating a duplicate.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"lesson_id":{"type":"number"},"evidence_refs":{"type":"array","items":{"type":"string"},"default":[]}},"required":["lesson_id"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.reinforceLesson(optInt64(args, "lesson_id", 0), getStringSlice(args, "evidence_refs"))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_get_details",
		Description: "Get details for one memory entity by id: event, decision, lesson, or session.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"entity_type":{"type":"string","enum":["event","decision","lesson","session"]},"id":{"type":"string"}},"required":["entity_type","id"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.getDetails(requireString(args, "entity_type"), requireString(args, "id"))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "memory_consolidate_lessons",
		Description: "Suggest lesson consolidation actions. MVP is dry-run only.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project_id":{"type":"string"},"dry_run":{"type":"boolean","default":true}},"required":["project_id"]}`),
	}, h(func(args map[string]interface{}) (interface{}, error) {
		return store.consolidateLessons(requireString(args, "project_id"), optBool(args, "dry_run", true))
	}))

}


func toolHandler(fn func(args map[string]interface{}) (interface{}, error)) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]interface{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return toolError("failed to parse arguments: " + err.Error()), nil
			}
		} else {
			args = make(map[string]interface{})
		}
		result, err := fn(args)
		if err != nil {
			return toolError(err.Error()), nil
		}
		jsonBytes, jsonErr := json.MarshalIndent(result, "", "  ")
		if jsonErr != nil {
			return toolError("JSON serialization error: " + jsonErr.Error()), nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(jsonBytes)}},
		}, nil
	}
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

func requireString(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func optString(args map[string]interface{}, key string) *string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

func optStringDefault(args map[string]interface{}, key string, defaultVal string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	if s, ok := v.(string); ok {
		return s
	}
	return defaultVal
}

func optBool(args map[string]interface{}, key string, defaultVal bool) bool {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return defaultVal
}

func optInt(args map[string]interface{}, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return defaultVal
}

func optInt64(args map[string]interface{}, key string, defaultVal int64) int64 {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return defaultVal
}

func getStringSlice(args map[string]interface{}, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return []string{}
	}
	arr, ok := v.([]interface{})
	if !ok {
		return []string{}
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
