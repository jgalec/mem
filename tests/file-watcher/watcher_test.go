package filewatcher

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "filewatcher-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestWatcher_FileCreated(t *testing.T) {
	dir := tempDir(t)
	w := New(Config{Dir: dir, Interval: 50 * time.Millisecond, Debounce: 50 * time.Millisecond})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	writeFile(t, filepath.Join(dir, "new.go"), "package main")

	events := w.CollectEvents(500 * time.Millisecond)
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	if events[0].Kind != EventCreated {
		t.Fatalf("expected EventCreated, got %s", events[0].Kind)
	}
}

func TestWatcher_FileModified(t *testing.T) {
	dir := tempDir(t)
	fp := filepath.Join(dir, "modify.go")
	writeFile(t, fp, "v1")

	w := New(Config{Dir: dir, Interval: 50 * time.Millisecond, Debounce: 50 * time.Millisecond})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	events := w.CollectEvents(200 * time.Millisecond)
	_ = events

	writeFile(t, fp, "v2")

	events = w.CollectEvents(500 * time.Millisecond)
	hasMod := false
	for _, e := range events {
		if e.Kind == EventModified {
			hasMod = true
			break
		}
	}
	if !hasMod {
		t.Fatal("expected a file_modified event")
	}
}

func TestWatcher_FileDeleted(t *testing.T) {
	dir := tempDir(t)
	fp := filepath.Join(dir, "remove.go")
	writeFile(t, fp, "x")

	w := New(Config{Dir: dir, Interval: 50 * time.Millisecond, Debounce: 50 * time.Millisecond})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	events := w.CollectEvents(200 * time.Millisecond)
	_ = events

	removeFile(t, fp)

	events = w.CollectEvents(500 * time.Millisecond)
	hasDel := false
	for _, e := range events {
		if e.Kind == EventDeleted {
			hasDel = true
			break
		}
	}
	if !hasDel {
		t.Fatal("expected a file_deleted event")
	}
}

func TestWatcher_ExtFilter(t *testing.T) {
	dir := tempDir(t)
	writeFile(t, filepath.Join(dir, "a.go"), "go file")
	writeFile(t, filepath.Join(dir, "b.txt"), "text file")

	w := New(Config{Dir: dir, Interval: 50 * time.Millisecond, Debounce: 50 * time.Millisecond, Exts: []string{".go"}})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	events := w.CollectEvents(500 * time.Millisecond)

	for _, e := range events {
		if filepath.Ext(e.Path) != ".go" {
			t.Fatalf("expected only .go files, got %s", e.Path)
		}
	}
}

func TestWatcher_Debounce(t *testing.T) {
	dir := tempDir(t)
	fp := filepath.Join(dir, "debounce.go")
	writeFile(t, fp, "a")

	w := New(Config{Dir: dir, Interval: 30 * time.Millisecond, Debounce: 200 * time.Millisecond})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	events := w.CollectEvents(150 * time.Millisecond)
	_ = events

	writeFile(t, fp, "b")
	time.Sleep(10 * time.Millisecond)
	writeFile(t, fp, "c")

	events = w.CollectEvents(500 * time.Millisecond)
	modCount := 0
	for _, e := range events {
		if e.Kind == EventModified {
			modCount++
		}
	}
	if modCount > 1 {
		t.Fatalf("expected at most 1 modified event after debounce, got %d", modCount)
	}
}

func TestWatcher_Subdirectories(t *testing.T) {
	dir := tempDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "sub", "nested"), 0755); err != nil {
		t.Fatal(err)
	}

	w := New(Config{Dir: dir, Interval: 50 * time.Millisecond, Debounce: 50 * time.Millisecond})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	writeFile(t, filepath.Join(dir, "sub", "nested", "deep.go"), "nested content")

	events := w.CollectEvents(500 * time.Millisecond)
	hasCreated := false
	for _, e := range events {
		if e.Kind == EventCreated && filepath.Base(e.Path) == "deep.go" {
			hasCreated = true
			break
		}
	}
	if !hasCreated {
		t.Fatal("expected created event for nested file")
	}
}

func TestWatcher_Stop(t *testing.T) {
	dir := tempDir(t)
	w := New(Config{Dir: dir, Interval: 50 * time.Millisecond})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	w.Stop()

	select {
	case _, ok := <-w.Events():
		if ok {
			t.Fatal("expected channel to be closed after Stop")
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func TestWatcher_CollectEventsOrder(t *testing.T) {
	dir := tempDir(t)
	for _, name := range []string{"a.go", "c.go", "b.go"} {
		writeFile(t, filepath.Join(dir, name), "content")
	}

	w := New(Config{Dir: dir, Interval: 50 * time.Millisecond, Debounce: 50 * time.Millisecond})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	events := w.CollectEvents(500 * time.Millisecond)
	if !sort.SliceIsSorted(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	}) {
		t.Fatal("events should be sorted by timestamp")
	}
}
