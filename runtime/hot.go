package runtime

import (
	"sort"
	"sync"
	"time"
)

type HotItem struct {
	ID            int64
	Title         string
	Score         float64
	Occurrences   int
	Confidence    float64
	LastUsed      time.Time
	LastReinforced time.Time
	Data          map[string]interface{}
}

type hotEntry struct {
	item HotItem
}

type HotTracker struct {
	mu       sync.RWMutex
	items    []hotEntry
	byID     map[int64]int
	maxSize  int
}

func NewHotTracker(maxSize int) *HotTracker {
	if maxSize < 1 {
		maxSize = 50
	}
	return &HotTracker{
		items:  make([]hotEntry, 0, maxSize),
		byID:   make(map[int64]int),
		maxSize: maxSize,
	}
}

func (h *HotTracker) Add(item HotItem) {
	if item.ID == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.addLocked(item)
}

func (h *HotTracker) AddFromRow(row map[string]interface{}) {
	item := HotItemFromRow(row)
	if item.ID == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.addLocked(item)
}

func (h *HotTracker) addLocked(item HotItem) {
	item.Score = h.computeScore(item)
	if idx, exists := h.byID[item.ID]; exists {
		h.items[idx].item = item
		h.resortLocked()
		return
	}
	if len(h.items) >= h.maxSize {
		last := h.items[len(h.items)-1]
		if item.Score <= last.item.Score {
			return
		}
		delete(h.byID, last.item.ID)
		h.items[len(h.items)-1] = hotEntry{item: item}
	} else {
		h.items = append(h.items, hotEntry{item: item})
	}
	h.resortLocked()
	for i := range h.items {
		h.byID[h.items[i].item.ID] = i
	}
}

func (h *HotTracker) resortLocked() {
	sort.Slice(h.items, func(i, j int) bool {
		if h.items[i].item.Score != h.items[j].item.Score {
			return h.items[i].item.Score > h.items[j].item.Score
		}
		if h.items[i].item.Occurrences != h.items[j].item.Occurrences {
			return h.items[i].item.Occurrences > h.items[j].item.Occurrences
		}
		return h.items[i].item.LastUsed.After(h.items[j].item.LastUsed)
	})
}

func (h *HotTracker) computeScore(item HotItem) float64 {
	score := item.Confidence * float64(item.Occurrences)
	if score < 1.0 {
		score = 1.0
	}
	hoursSinceUse := time.Since(item.LastUsed).Hours()
	if hoursSinceUse > 0 {
		score *= (1.0 + 1.0/hoursSinceUse)
	}
	hoursSinceReinforce := time.Since(item.LastReinforced).Hours()
	if hoursSinceReinforce > 0 {
		score *= (1.0 + 0.5/hoursSinceReinforce)
	}
	return score
}

func (h *HotTracker) Bump(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if idx, ok := h.byID[id]; ok {
		h.items[idx].item.LastUsed = time.Now()
		h.items[idx].item.Score = h.computeScore(h.items[idx].item)
		h.resortLocked()
		for i := range h.items {
			h.byID[h.items[i].item.ID] = i
		}
	}
}

func (h *HotTracker) TopN(n int) []HotItem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if n > len(h.items) {
		n = len(h.items)
	}
	result := make([]HotItem, n)
	for i := 0; i < n; i++ {
		result[i] = h.items[i].item
	}
	return result
}

func (h *HotTracker) Get(id int64) (HotItem, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if idx, ok := h.byID[id]; ok {
		return h.items[idx].item, true
	}
	return HotItem{}, false
}

func (h *HotTracker) Remove(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if idx, ok := h.byID[id]; ok {
		delete(h.byID, id)
		h.items = append(h.items[:idx], h.items[idx+1:]...)
		for i := range h.items {
			h.byID[h.items[i].item.ID] = i
		}
	}
}

func (h *HotTracker) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.items)
}

func (h *HotTracker) All() []HotItem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]HotItem, len(h.items))
	for i := range h.items {
		result[i] = h.items[i].item
	}
	return result
}

func HotItemFromRow(row map[string]interface{}) HotItem {
	item := HotItem{Data: row}
	if v, ok := row["id"]; ok {
		switch id := v.(type) {
		case int64:
			item.ID = id
		case float64:
			item.ID = int64(id)
		case int:
			item.ID = int64(id)
		}
	}
	if v, ok := row["title"].(string); ok {
		item.Title = v
	}
	if v, ok := row["confidence"].(float64); ok {
		item.Confidence = v
	} else if v, ok := row["confidence"].(int64); ok {
		item.Confidence = float64(v)
	} else {
		item.Confidence = 1.0
	}
	switch v := row["occurrences"].(type) {
	case int64:
		item.Occurrences = int(v)
	case float64:
		item.Occurrences = int(v)
	case int:
		item.Occurrences = v
	default:
		item.Occurrences = 1
	}
	if v, ok := row["last_used_at"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			item.LastUsed = t
		}
	}
	if item.LastUsed.IsZero() {
		item.LastUsed = time.Now()
	}
	if v, ok := row["last_reinforced_at"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			item.LastReinforced = t
		}
	}
	return item
}
