package runtime

import (
	"testing"
	"time"
)

func TestTTLCacheBasic(t *testing.T) {
	c := NewTTLCache[string, int](50 * time.Millisecond)
	defer c.Stop()

	c.Set("a", 1)
	c.Set("b", 2)

	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Errorf("expected 1, got %v, ok=%v", v, ok)
	}

	v, ok = c.Get("b")
	if !ok || v != 2 {
		t.Errorf("expected 2, got %v, ok=%v", v, ok)
	}

	_, ok = c.Get("c")
	if ok {
		t.Error("expected miss for 'c'")
	}

	if c.Len() != 2 {
		t.Errorf("expected len 2, got %d", c.Len())
	}
}

func TestTTLCacheExpiry(t *testing.T) {
	c := NewTTLCache[string, int](30 * time.Millisecond)
	defer c.Stop()

	c.Set("x", 42)
	time.Sleep(60 * time.Millisecond)

	_, ok := c.Get("x")
	if ok {
		t.Error("expected expiry but got hit")
	}
}

func TestTTLCacheDelete(t *testing.T) {
	c := NewTTLCache[string, int](time.Hour)
	defer c.Stop()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Delete("a")

	if c.Len() != 1 {
		t.Errorf("expected len 1, got %d", c.Len())
	}
	_, ok := c.Get("a")
	if ok {
		t.Error("expected miss after delete")
	}
}

func TestTTLCacheInvalidate(t *testing.T) {
	c := NewTTLCache[string, int](time.Hour)
	defer c.Stop()

	c.Set("key:1", 1)
	c.Set("key:2", 2)
	c.Set("other:3", 3)

	c.Invalidate(func(k string) bool {
		return len(k) >= 4 && k[:4] == "key:"
	})

	if c.Len() != 1 {
		t.Errorf("expected len 1 after invalidate, got %d", c.Len())
	}
	_, ok := c.Get("other:3")
	if !ok {
		t.Error("expected 'other:3' to survive")
	}
}

func TestTTLCacheSetTTL(t *testing.T) {
	c := NewTTLCache[string, int](10 * time.Minute)
	defer c.Stop()

	c.SetTTL("short", 1, 20*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	_, ok := c.Get("short")
	if ok {
		t.Error("expected expiry for custom TTL")
	}
}

func TestHotTrackerAddAndTopN(t *testing.T) {
	h := NewHotTracker(10)

	now := time.Now()
	h.Add(HotItem{ID: 1, Title: "a", Confidence: 1.0, Occurrences: 5, LastUsed: now, LastReinforced: now})
	h.Add(HotItem{ID: 2, Title: "b", Confidence: 1.5, Occurrences: 3, LastUsed: now, LastReinforced: now})
	h.Add(HotItem{ID: 3, Title: "c", Confidence: 0.8, Occurrences: 10, LastUsed: now, LastReinforced: now})

	top := h.TopN(2)
	if len(top) != 2 {
		t.Fatalf("expected 2 items, got %d", len(top))
	}
	if top[0].ID == 0 || top[1].ID == 0 {
		t.Error("IDs should not be zero")
	}
}

func TestHotTrackerMaxSize(t *testing.T) {
	h := NewHotTracker(3)
	now := time.Now()

	for i := int64(1); i <= 5; i++ {
		h.Add(HotItem{ID: i, Title: "x", Confidence: float64(i), Occurrences: 1, LastUsed: now, LastReinforced: now})
	}

	if h.Len() > 3 {
		t.Errorf("expected max 3, got %d", h.Len())
	}
}

func TestHotTrackerBump(t *testing.T) {
	h := NewHotTracker(10)
	now := time.Now()

	h.Add(HotItem{ID: 1, Title: "a", Confidence: 1.0, Occurrences: 1, LastUsed: now, LastReinforced: now})
	h.Add(HotItem{ID: 2, Title: "b", Confidence: 1.0, Occurrences: 1, LastUsed: now, LastReinforced: now})

	h.Bump(1)
	top := h.TopN(1)
	if len(top) == 0 || top[0].ID != 1 {
		t.Errorf("expected ID 1 to be top after bump, got %v", top)
	}
}

func TestHotTrackerRemove(t *testing.T) {
	h := NewHotTracker(10)
	now := time.Now()

	h.Add(HotItem{ID: 1, Title: "a", Confidence: 1.0, Occurrences: 1, LastUsed: now, LastReinforced: now})
	h.Add(HotItem{ID: 2, Title: "b", Confidence: 1.0, Occurrences: 1, LastUsed: now, LastReinforced: now})

	h.Remove(1)
	if h.Len() != 1 {
		t.Errorf("expected len 1, got %d", h.Len())
	}
	_, ok := h.Get(1)
	if ok {
		t.Error("expected miss after remove")
	}
}

func TestHotItemFromRow(t *testing.T) {
	row := map[string]interface{}{
		"id":                 int64(42),
		"title":              "test",
		"confidence":         float64(1.5),
		"occurrences":        int64(7),
		"last_used_at":       "2024-01-15T10:30:00Z",
		"last_reinforced_at": "2024-01-14T08:00:00Z",
	}

	item := HotItemFromRow(row)
	if item.ID != 42 {
		t.Errorf("expected ID 42, got %d", item.ID)
	}
	if item.Title != "test" {
		t.Errorf("expected title 'test', got '%s'", item.Title)
	}
	if item.Confidence != 1.5 {
		t.Errorf("expected confidence 1.5, got %f", item.Confidence)
	}
	if item.Occurrences != 7 {
		t.Errorf("expected occurrences 7, got %d", item.Occurrences)
	}
}

func TestSnapshotStoreAndGet(t *testing.T) {
	r := New(DefaultConfig())
	defer r.Shutdown()

	data := map[string]interface{}{"key": "value", "num": 42}
	key, err := r.StoreSnapshot("proj1", data)
	if err != nil {
		t.Fatalf("StoreSnapshot: %v", err)
	}
	if key == "" {
		t.Fatal("expected non-empty snapshot key")
	}

	got, ok := r.GetSnapshot("proj1", key)
	if !ok {
		t.Fatal("expected snapshot hit")
	}
	if got["key"] != "value" {
		t.Errorf("expected 'value', got %v", got["key"])
	}
}

func TestSnapshotInvalidation(t *testing.T) {
	r := New(DefaultConfig())
	defer r.Shutdown()

	key1, _ := r.StoreSnapshot("proj1", map[string]interface{}{"a": 1})
	key2, _ := r.StoreSnapshot("proj2", map[string]interface{}{"b": 2})

	r.InvalidProjectSnapshots("proj1")

	_, ok := r.GetSnapshot("proj1", key1)
	if ok {
		t.Error("expected proj1 snapshot to be invalidated")
	}
	_, ok = r.GetSnapshot("proj2", key2)
	if !ok {
		t.Error("expected proj2 snapshot to survive")
	}
}

func TestRuntimeWriteQueue(t *testing.T) {
	var flushed []WriteOp
	cfg := DefaultConfig()
	cfg.WriteQueueSize = 4
	cfg.FlushInterval = 100 * time.Millisecond

	r := New(cfg)
	defer r.Shutdown()

	r.SetOnFlush(func(ops []WriteOp) error {
		flushed = append(flushed, ops...)
		return nil
	})

	for i := 0; i < 3; i++ {
		<-r.EnqueueWrite("INSERT INTO test VALUES (?)", i)
	}
	time.Sleep(200 * time.Millisecond)

	if len(flushed) != 3 {
		t.Errorf("expected 3 flushed, got %d", len(flushed))
	}
}
