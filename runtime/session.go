package runtime

import (
	"strings"
	"time"
)

type WorkingContext struct {
	Summary       string
	ActiveWindow  []string
	RecentActions []string
	UpdatedAt     time.Time
}

func (r *Runtime) GetWorkingContext(sessionId string) *WorkingContext {
	key := "working_context:" + sessionId
	if v, ok := r.sessionState.Get(key); ok {
		if wc, ok := v["_wc"].(*WorkingContext); ok {
			return wc
		}
	}
	return nil
}

func (r *Runtime) SetWorkingContext(sessionId string, wc *WorkingContext) {
	key := "working_context:" + sessionId
	wc.UpdatedAt = time.Now()
	r.sessionState.Set(key, map[string]interface{}{"_wc": wc})
}

func (r *Runtime) AddTemporarySummary(sessionId string, summary string) {
	key := "temp_summary:" + sessionId
	r.sessionState.SetTTL(key, map[string]interface{}{"summary": summary}, 1*time.Hour)
}

func (r *Runtime) GetTemporarySummary(sessionId string) string {
	key := "temp_summary:" + sessionId
	if v, ok := r.sessionState.Get(key); ok {
		if s, ok := v["summary"].(string); ok {
			return s
		}
	}
	return ""
}

func (r *Runtime) StoreTransientState(sessionId, stateKey string, value interface{}) {
	key := "transient:" + sessionId + ":" + stateKey
	r.sessionState.SetTTL(key, map[string]interface{}{"value": value}, 1*time.Hour)
}

func (r *Runtime) GetTransientState(sessionId, stateKey string) (interface{}, bool) {
	key := "transient:" + sessionId + ":" + stateKey
	if v, ok := r.sessionState.Get(key); ok {
		if val, exists := v["value"]; exists {
			return val, true
		}
	}
	return nil, false
}

func (r *Runtime) ClearSessionState(sessionId string) {
	r.sessionState.Invalidate(func(key string) bool {
		return strings.HasSuffix(key, ":"+sessionId) ||
			(len(key) > 10+len(sessionId) && key[:10] == "transient:" && key[10:10+len(sessionId)] == sessionId)
	})
}
