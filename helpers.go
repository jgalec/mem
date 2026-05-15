package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

func scanRowToMap(rows *sql.Rows) (map[string]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	values := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	for i, col := range cols {
		v := values[i]
		switch val := v.(type) {
		case []byte:
			m[col] = string(val)
		case nil:
			m[col] = nil
		default:
			m[col] = val
		}
	}
	return m, nil
}

func isoNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func hydrateJsonColumns(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	for _, key := range []string{"evidence_refs", "alternatives_considered", "tags"} {
		if s, ok := out[key].(string); ok {
			out[key] = parseJSONArray(s)
		}
	}
	return out
}

func parseJSONArray(s string) interface{} {
	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return s
	}
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if str, ok := v.(string); ok {
			result = append(result, str)
		}
	}
	if len(result) > 0 {
		return result
	}
	return []string{}
}

func expandLesson(m map[string]interface{}, score int) map[string]interface{} {
	expanded := hydrateJsonColumns(m)
	expanded["score"] = score
	return expanded
}

func compactLesson(m map[string]interface{}, score int) map[string]interface{} {
	return map[string]interface{}{
		"id":                m["id"],
		"title":             m["title"],
		"description":       m["description"],
		"status":            m["status"],
		"tags":              parseJSONArray(strVal(m, "tags")),
		"confidence":        m["confidence"],
		"occurrences":       m["occurrences"],
		"last_reinforced_at": m["last_reinforced_at"],
		"score":             score,
	}
}

func lessonScore(row map[string]interface{}, query string, tags []string) int {
	haystack := strings.ToLower(strVal(row, "title") + " " + strVal(row, "description") + " " + strVal(row, "content"))
	score := 0
	if query != "" {
		for _, term := range strings.Fields(query) {
			if strings.Contains(haystack, term) {
				score += 2
			}
		}
	} else {
		score = 1
	}
	lessonTags := toStringSlice(row, "tags")
	lessonTagSet := make(map[string]bool)
	for _, t := range lessonTags {
		lessonTagSet[strings.ToLower(t)] = true
	}
	for _, t := range tags {
		if lessonTagSet[strings.ToLower(t)] {
			score += 3
		}
	}
	if strVal(row, "status") == "consolidated" {
		score += 1
	}
	return score
}

func nullableRedact(value string) interface{} {
	if value == "" {
		return nil
	}
	return redact(value)
}

func redactList(values []string) []string {
	if values == nil {
		return []string{}
	}
	result := make([]string, len(values))
	for i, v := range values {
		result[i] = redact(v)
	}
	return result
}

func redact(value string) string {
	re := regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password)\s*[:=]\s*[^\s,;]+`)
	value = re.ReplaceAllString(value, "$1=[REDACTED]")
	re2 := regexp.MustCompile(`\b(sk-[A-Za-z0-9_-]{12,}|ghp_[A-Za-z0-9_]{12,})\b`)
	value = re2.ReplaceAllString(value, "[REDACTED]")
	return value
}

func mergeStrings(first, second []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range first {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range second {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func tagOverlap(left, right string) int {
	leftTags := toStringMap(left)
	rightTags := parseTagList(right)
	count := 0
	for _, t := range rightTags {
		if leftTags[strings.ToLower(t)] {
			count++
		}
	}
	return count
}

func toStringMap(s string) map[string]bool {
	m := make(map[string]bool)
	for _, t := range parseTagList(s) {
		m[strings.ToLower(t)] = true
	}
	return m
}

func parseTagList(s string) []string {
	if s == "" {
		return []string{}
	}
	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return []string{}
	}
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if str, ok := v.(string); ok {
			result = append(result, str)
		}
	}
	return result
}

func normalizeTitle(value string) string {
	re := regexp.MustCompile(`\s+`)
	return strings.ToLower(re.ReplaceAllString(strings.TrimSpace(value), " "))
}

func normalizeNamespace(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(strings.TrimSpace(value), "/")
	var filtered []string
	for i, p := range parts {
		if i >= 3 {
			break
		}
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.ToLower(p)
		re := regexp.MustCompile(`[^a-z0-9\s-]`)
		p = re.ReplaceAllString(p, "")
		p = strings.TrimSpace(p)
		re2 := regexp.MustCompile(`\s+`)
		p = re2.ReplaceAllString(p, "-")
		re3 := regexp.MustCompile(`-+`)
		p = re3.ReplaceAllString(p, "-")
		p = strings.Trim(p, "-")
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	joined := strings.Join(filtered, "/")
	if len(joined) > 128 {
		joined = joined[:128]
	}
	if joined == "" {
		return ""
	}
	return joined
}

func toFtsQuery(query string) string {
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return ""
	}
	terms := make([]string, len(parts))
	for i, p := range parts {
		escaped := strings.ReplaceAll(p, `"`, `""`)
		terms[i] = `"` + escaped + `"*`
	}
	return strings.Join(terms, " AND ")
}

func intVal(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case int64:
		return int(v)
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func floatVal(m map[string]interface{}, key string, defaultVal float64) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	}
	return defaultVal
}

func toStringSlice(m map[string]interface{}, key string) []string {
	v := m[key]
	if v == nil {
		return []string{}
	}
	if s, ok := v.(string); ok {
		return parseTagList(s)
	}
	if arr, ok := v.([]string); ok {
		return arr
	}
	if arr, ok := v.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return []string{}
}
