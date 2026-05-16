package logrotator

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Config struct {
	MaxSize    int64
	MaxBackups int
	Compress   bool
}

type Rotator struct {
	cfg Config
	mu  sync.Mutex
}

func New(cfg Config) *Rotator {
	return &Rotator{cfg: cfg}
}

func (r *Rotator) Rotate(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cfg.MaxSize <= 0 {
		return nil
	}

	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	if fi.Size() < r.cfg.MaxSize {
		return nil
	}

	backups, err := r.listBackups(path)
	if err != nil {
		return err
	}

	if r.cfg.MaxBackups > 0 {
		for _, idx := range backups {
			if idx >= r.cfg.MaxBackups {
				if err := r.removeBackup(path, idx); err != nil {
					return err
				}
			}
		}
	}

	for i := r.cfg.MaxBackups - 1; i > 0; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
		srcGz := src + ".gz"
		dstGz := dst + ".gz"
		if _, err := os.Stat(srcGz); err == nil {
			if err := os.Rename(srcGz, dstGz); err != nil {
				return err
			}
		}
	}

	rotatedName := fmt.Sprintf("%s.1", path)
	if err := os.Rename(path, rotatedName); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	file.Close()

	if r.cfg.Compress {
		for i := 1; i <= r.cfg.MaxBackups; i++ {
			bp := fmt.Sprintf("%s.%d", path, i)
			if info, err := os.Stat(bp); err == nil && !strings.HasSuffix(bp, ".gz") {
				if info.Size() > 0 {
					if err := r.compressFile(bp); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func (r *Rotator) listBackups(path string) ([]int, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var indices []int
	prefix := base + "."
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := name[len(prefix):]
		suffix = strings.TrimSuffix(suffix, ".gz")
		var idx int
		if _, err := fmt.Sscanf(suffix, "%d", &idx); err == nil && idx > 0 {
			indices = append(indices, idx)
		}
	}

	sort.Sort(sort.Reverse(sort.IntSlice(indices)))
	return indices, nil
}

func (r *Rotator) removeBackup(path string, idx int) error {
	bp := fmt.Sprintf("%s.%d", path, idx)
	if _, err := os.Stat(bp); err == nil {
		if err := os.Remove(bp); err != nil {
			return err
		}
	}
	bpGz := bp + ".gz"
	if _, err := os.Stat(bpGz); err == nil {
		if err := os.Remove(bpGz); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rotator) compressFile(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	gzPath := path + ".gz"
	dst, err := os.Create(gzPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	gw := gzip.NewWriter(dst)
	if _, err := io.Copy(gw, src); err != nil {
		gw.Close()
		dst.Close()
		os.Remove(gzPath)
		return err
	}

	if err := gw.Close(); err != nil {
		dst.Close()
		os.Remove(gzPath)
		return err
	}
	dst.Close()
	src.Close()

	return os.Remove(path)
}
