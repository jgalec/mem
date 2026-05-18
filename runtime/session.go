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

func (r *Runtime) ClearSessionState(sessionId string) {
	r.sessionState.Invalidate(func(key string) bool {
		return strings.HasSuffix(key, ":"+sessionId) ||
			strings.Contains(key, "transient:"+sessionId+":")
	})
}
