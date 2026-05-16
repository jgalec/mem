package filewatcher

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type EventKind string

const (
	EventCreated  EventKind = "file_created"
	EventModified EventKind = "file_modified"
	EventDeleted  EventKind = "file_deleted"
)

type Event struct {
	Path      string
	Kind      EventKind
	Timestamp time.Time
}

type Watcher struct {
	dir       string
	interval  time.Duration
	debounceWindow time.Duration
	extFilter      map[string]bool

	mu           sync.Mutex
	hashes       map[string]string
	seen         map[string]bool
	debounceSeen map[string]time.Time
	running  bool
	stopCh   chan struct{}
	eventsCh chan Event
}

type Config struct {
	Dir      string
	Interval time.Duration
	Debounce time.Duration
	Exts     []string
}

func New(cfg Config) *Watcher {
	if cfg.Interval <= 0 {
		cfg.Interval = 500 * time.Millisecond
	}
	if cfg.Debounce <= 0 {
		cfg.Debounce = 200 * time.Millisecond
	}

	extFilter := make(map[string]bool)
	for _, ext := range cfg.Exts {
		extFilter[ext] = true
	}

	return &Watcher{
		dir:       cfg.Dir,
		interval:  cfg.Interval,
		debounceWindow: cfg.Debounce,
		extFilter:      extFilter,
		hashes:         make(map[string]string),
		seen:           make(map[string]bool),
		debounceSeen:   make(map[string]time.Time),
		stopCh:         make(chan struct{}),
		eventsCh:       make(chan Event, 256),
	}
}

func (w *Watcher) Events() <-chan Event {
	return w.eventsCh
}

func (w *Watcher) Start() error {
	if w.running {
		return nil
	}

	hashes, err := w.scan()
	if err != nil {
		return err
	}

	w.mu.Lock()
	w.hashes = hashes
	for p := range hashes {
		w.seen[p] = true
	}
	w.running = true
	w.mu.Unlock()

	go w.poll()
	return nil
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()
	close(w.stopCh)
}

func (w *Watcher) poll() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			close(w.eventsCh)
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *Watcher) check() {
	current, err := w.scan()
	if err != nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()

	for path, hash := range current {
		if !w.seen[path] {
			w.seen[path] = true
			w.hashes[path] = hash
			w.emitLocked(Event{Path: path, Kind: EventCreated, Timestamp: now})
		} else if w.hashes[path] != hash {
			w.hashes[path] = hash
			if d, ok := w.debounceSeen[path]; ok && now.Sub(d) < w.debounceWindow {
				continue
			}
			w.debounceSeen[path] = now
			w.emitLocked(Event{Path: path, Kind: EventModified, Timestamp: now})
		}
	}

	for path := range w.seen {
		if _, ok := current[path]; !ok {
			delete(w.seen, path)
			delete(w.hashes, path)
			delete(w.debounceSeen, path)
			w.emitLocked(Event{Path: path, Kind: EventDeleted, Timestamp: now})
		}
	}
}

func (w *Watcher) emitLocked(e Event) {
	select {
	case w.eventsCh <- e:
	default:
	}
}

func (w *Watcher) scan() (map[string]string, error) {
	hashes := make(map[string]string)
	err := filepath.WalkDir(w.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if len(w.extFilter) > 0 {
			ext := filepath.Ext(path)
			if !w.extFilter[ext] {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		h := sha256.Sum256(data)
		hashes[path] = hex.EncodeToString(h[:])
		return nil
	})
	return hashes, err
}

func (w *Watcher) CollectEvents(timeout time.Duration) []Event {
	var events []Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case e, ok := <-w.Events():
			if !ok {
				sort.Slice(events, func(i, j int) bool {
					return events[i].Timestamp.Before(events[j].Timestamp)
				})
				return events
			}
			events = append(events, e)
		case <-timer.C:
			sort.Slice(events, func(i, j int) bool {
				return events[i].Timestamp.Before(events[j].Timestamp)
			})
			return events
		}
	}
}
