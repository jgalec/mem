package runtime

import (
	"sync"
	"time"
)

type Runtime struct {
	startupCache   *TTLCache[string, map[string]interface{}]
	retrievalCache *TTLCache[string, []map[string]interface{}]
	hotLessons     *HotTracker
	sessionState   *TTLCache[string, map[string]interface{}]
	snapshots      map[string][]byte
	snapshotMu     sync.RWMutex
	writeQueue     chan WriteOp
	flushDone      chan struct{}
	onFlush        func([]WriteOp) error
	config         Config
}

type Config struct {
	StartupCacheTTL  time.Duration
	RetrievalCacheTTL time.Duration
	SessionStateTTL  time.Duration
	HotLessonMaxSize int
	WriteQueueSize   int
	FlushInterval    time.Duration
}

func DefaultConfig() Config {
	return Config{
		StartupCacheTTL:   5 * time.Minute,
		RetrievalCacheTTL: 2 * time.Minute,
		SessionStateTTL:   24 * time.Hour,
		HotLessonMaxSize:  50,
		WriteQueueSize:    64,
		FlushInterval:     500 * time.Millisecond,
	}
}

func New(config Config) *Runtime {
	if config.HotLessonMaxSize == 0 {
		config.HotLessonMaxSize = 50
	}
	if config.WriteQueueSize == 0 {
		config.WriteQueueSize = 64
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 500 * time.Millisecond
	}

	r := &Runtime{
		startupCache:   NewTTLCache[string, map[string]interface{}](config.StartupCacheTTL),
		retrievalCache: NewTTLCache[string, []map[string]interface{}](config.RetrievalCacheTTL),
		hotLessons:     NewHotTracker(config.HotLessonMaxSize),
		sessionState:   NewTTLCache[string, map[string]interface{}](config.SessionStateTTL),
		snapshots:      make(map[string][]byte),
		writeQueue: make(chan WriteOp, config.WriteQueueSize),
		flushDone:      make(chan struct{}),
		config:         config,
	}

	go r.flushLoop()
	return r
}

func (r *Runtime) StartupCache() *TTLCache[string, map[string]interface{}] { return r.startupCache }
func (r *Runtime) RetrievalCache() *TTLCache[string, []map[string]interface{}] { return r.retrievalCache }
func (r *Runtime) HotLessons() *HotTracker                                   { return r.hotLessons }
func (r *Runtime) SessionState() *TTLCache[string, map[string]interface{}]  { return r.sessionState }
func (r *Runtime) Snapshots() map[string][]byte                              { return r.snapshots }
func (r *Runtime) SnapshotCount() int {
	r.snapshotMu.RLock()
	defer r.snapshotMu.RUnlock()
	return len(r.snapshots)
}

func (r *Runtime) Shutdown() {
	close(r.writeQueue)
	<-r.flushDone
	r.startupCache.Stop()
	r.retrievalCache.Stop()
	r.sessionState.Stop()
}

func (r *Runtime) InvalidateStartupCache(projectId string) {
	r.startupCache.Invalidate(func(key string) bool {
		prefix := "startup_context:" + projectId
		return len(key) >= len(prefix) && key[:len(prefix)] == prefix
	})
}

func (r *Runtime) InvalidateRetrievalCache(projectId string) {
	prefix := projectId + ":"
	r.retrievalCache.Invalidate(func(key string) bool {
		return len(key) > len(prefix) && key[:len(prefix)] == prefix
	})
}

