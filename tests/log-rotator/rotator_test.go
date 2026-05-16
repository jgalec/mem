package logrotator

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func tempFile(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, name)
}

func writeFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if size > 0 {
		if _, err := f.Write(make([]byte, size)); err != nil {
			f.Close()
			t.Fatalf("write: %v", err)
		}
	}
	f.Close()
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestNoRotationUnderSize(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 500)

	r := New(Config{MaxSize: 1024, MaxBackups: 3})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fi := fileSize(t, path); fi != 500 {
		t.Errorf("file was modified: size=%d, want 500", fi)
	}
	if fileExists(path + ".1") {
		t.Error("backup created but should not have been")
	}
}

func TestRotationCreatesBackup(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 2048)

	r := New(Config{MaxSize: 1024, MaxBackups: 3})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fileExists(path + ".1") {
		t.Error("backup file .1 not created")
	}
	if fi := fileSize(t, path + ".1"); fi != 2048 {
		t.Errorf("backup size=%d, want 2048", fi)
	}
	if fi := fileSize(t, path); fi != 0 {
		t.Errorf("current log size=%d, want 0 (empty)", fi)
	}
}

func TestShiftExistingBackups(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 2000)
	writeFile(t, path+".1", 1000)
	writeFile(t, path+".2", 500)

	r := New(Config{MaxSize: 1024, MaxBackups: 3})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fileExists(path + ".1") {
		t.Error("missing .1")
	}
	if !fileExists(path + ".2") {
		t.Error("missing .2")
	}
	if !fileExists(path + ".3") {
		t.Error("missing .3")
	}

	if fi := fileSize(t, path+".2"); fi != 1000 {
		t.Errorf("backup .2 size=%d, want 1000", fi)
	}
	if fi := fileSize(t, path+".3"); fi != 500 {
		t.Errorf("backup .3 size=%d, want 500", fi)
	}
}

func TestMaxBackupsEnforced(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 2000)
	writeFile(t, path+".1", 100)
	writeFile(t, path+".2", 100)
	writeFile(t, path+".3", 100)
	writeFile(t, path+".4", 100)

	r := New(Config{MaxSize: 1024, MaxBackups: 3})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 1; i <= 3; i++ {
		if !fileExists(path + suffix(i)) {
			t.Errorf("backup .%d should exist", i)
		}
	}
	if fileExists(path + ".4") {
		t.Error("backup .4 should have been removed")
	}
}

func TestCompressBackups(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 2000)

	r := New(Config{MaxSize: 1024, MaxBackups: 2, Compress: true})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fileExists(path + ".1.gz") {
		t.Error("compressed backup not created")
	}
	if fileExists(path + ".1") {
		t.Error("uncompressed backup should have been removed")
	}

	f, err := os.Open(path + ".1.gz")
	if err != nil {
		t.Fatalf("open compressed: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("read compressed: %v", err)
	}
	if len(data) != 2000 {
		t.Errorf("decompressed size=%d, want 2000", len(data))
	}
}

func TestConcurrentSafety(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 5000)

	r := New(Config{MaxSize: 1024, MaxBackups: 5})

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.Rotate(path); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent rotate error: %v", err)
	}

	if fi := fileSize(t, path); fi != 0 {
		t.Errorf("current log should be empty after concurrent rotates, got size=%d", fi)
	}
}

func TestZeroMaxSizeNoop(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 99999)

	r := New(Config{MaxSize: 0, MaxBackups: 3})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fileExists(path + ".1") {
		t.Error("no rotation expected with MaxSize=0")
	}
}

func TestMissingFileError(t *testing.T) {
	r := New(Config{MaxSize: 1024, MaxBackups: 3})
	err := r.Rotate("/nonexistent/path/app.log")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCompressExistingOldBackups(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 2000)
	writeFile(t, path+".1", 1500)

	r := New(Config{MaxSize: 1024, MaxBackups: 3, Compress: true})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fileExists(path + ".1.gz") {
		t.Error("new backup should be compressed")
	}
	if !fileExists(path + ".2.gz") {
		t.Error("old backup should be compressed")
	}
	if fileExists(path + ".1") || fileExists(path + ".2") {
		t.Error("uncompressed backups should be removed")
	}
}

func TestCompressShiftsGzippedFiles(t *testing.T) {
	path := tempFile(t, "app.log")
	writeFile(t, path, 2000)
	writeFile(t, path+".1", 100)
	writeFile(t, path+".2", 100)
	// compress .1 manually
	data, _ := os.ReadFile(path + ".1")
	f, _ := os.Create(path + ".1.gz")
	w := gzip.NewWriter(f)
	w.Write(data)
	w.Close()
	f.Close()
	os.Remove(path + ".1")

	r := New(Config{MaxSize: 1024, MaxBackups: 3, Compress: true})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fileExists(path + ".1.gz") {
		t.Error("compressed .1.gz should exist")
	}
	if !fileExists(path + ".2.gz") {
		t.Error("old compressed .2.gz should exist (shifted from .1)")
	}
	if !fileExists(path + ".3.gz") {
		t.Error("old .3.gz should exist (shifted from .2)")
	}
}

func suffix(n int) string {
	return fmt.Sprintf(".%d", n)
}
