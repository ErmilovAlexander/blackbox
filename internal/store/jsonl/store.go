package jsonl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
	storepkg "github.com/ErmilovAlexander/blackbox/internal/store"
)

type Store struct {
	mu              sync.Mutex
	dir             string
	retention       time.Duration
	maxStoreBytes   int64
	maxSegmentBytes int64
	readOnly        bool
	closed          bool
	file            *os.File
	writer          *bufio.Writer
	segmentBytes    int64
}

var (
	ErrClosed   = errors.New("jsonl store is closed")
	ErrReadOnly = errors.New("jsonl store is read-only")
)

func Open(dir string, retention time.Duration, maxStoreBytes, maxSegmentBytes int64) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("data directory is required")
	}
	if maxSegmentBytes <= 0 {
		return nil, fmt.Errorf("maxSegmentBytes must be > 0")
	}
	if maxStoreBytes > 0 && maxSegmentBytes > maxStoreBytes {
		return nil, fmt.Errorf("maxSegmentBytes (%d) must not exceed maxStoreBytes (%d)", maxSegmentBytes, maxStoreBytes)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, retention: retention, maxStoreBytes: maxStoreBytes, maxSegmentBytes: maxSegmentBytes}
	if err := s.rotateLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

// OpenReadOnly opens an existing data directory without creating a segment.
// It is used by query commands so inspecting evidence never mutates it.
func OpenReadOnly(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("data directory is required")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("open data directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("data directory %q is not a directory", dir)
	}
	return &Store{dir: dir, readOnly: true}, nil
}

func (s *Store) Append(ctx context.Context, r model.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if s.readOnly {
		return ErrReadOnly
	}
	if s.file == nil || s.segmentBytes+int64(len(b)+1) > s.maxSegmentBytes {
		if err := s.rotateLocked(); err != nil {
			return err
		}
	}
	if _, err := s.writer.Write(b); err != nil {
		return err
	}
	if err := s.writer.WriteByte('\n'); err != nil {
		return err
	}
	if err := s.writer.Flush(); err != nil {
		return err
	}
	s.segmentBytes += int64(len(b) + 1)
	return s.enforceRetentionLocked(time.Now())
}

func (s *Store) Query(ctx context.Context, q storepkg.Query) ([]model.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}
	files, err := filepath.Glob(filepath.Join(s.dir, "segment-*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	out := make([]model.Record, 0, 128)
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		buf := make([]byte, 64*1024)
		sc.Buffer(buf, 16*1024*1024)
		for sc.Scan() {
			var r model.Record
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				_ = f.Close()
				return nil, fmt.Errorf("decode %s: %w", path, err)
			}
			if match(r, q) {
				out = append(out, r)
				if q.Limit > 0 && len(out) >= q.Limit {
					_ = f.Close()
					return out, nil
				}
			}
		}
		if err := sc.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		_ = f.Close()
	}
	return out, nil
}

func match(r model.Record, q storepkg.Query) bool {
	if !q.From.IsZero() && r.ObservedAt.Before(q.From) {
		return false
	}
	if !q.To.IsZero() && r.ObservedAt.After(q.To) {
		return false
	}
	if q.Namespace != "" && r.Namespace != q.Namespace {
		return false
	}
	if q.Kind != "" && !strings.EqualFold(r.Kind, q.Kind) {
		return false
	}
	if q.Name != "" && r.Name != q.Name {
		return false
	}
	if q.UID != "" && r.UID != q.UID {
		return false
	}
	return true
}

func (s *Store) rotateLocked() error {
	if s.writer != nil {
		_ = s.writer.Flush()
	}
	if s.file != nil {
		_ = s.file.Close()
	}
	name := filepath.Join(s.dir, fmt.Sprintf("segment-%020d.jsonl", time.Now().UnixNano()))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	s.file = f
	s.writer = bufio.NewWriterSize(f, 256*1024)
	s.segmentBytes = 0
	return nil
}

func (s *Store) enforceRetentionLocked(now time.Time) error {
	files, err := filepath.Glob(filepath.Join(s.dir, "segment-*.jsonl"))
	if err != nil {
		return err
	}
	type info struct {
		path string
		mod  time.Time
		size int64
	}
	items := make([]info, 0, len(files))
	var total int64
	for _, path := range files {
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		items = append(items, info{path: path, mod: st.ModTime(), size: st.Size()})
		total += st.Size()
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.Before(items[j].mod) })
	for _, item := range items {
		if s.file != nil && sameFile(item.path, s.file.Name()) {
			continue
		}
		expired := s.retention > 0 && now.Sub(item.mod) > s.retention
		overSize := s.maxStoreBytes > 0 && total > s.maxStoreBytes
		if !expired && !overSize {
			continue
		}
		if err := os.Remove(item.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		total -= item.size
	}
	return nil
}

func sameFile(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa == bb
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var errs []error
	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.file != nil {
		if err := s.file.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	s.writer = nil
	s.file = nil
	return errors.Join(errs...)
}

var _ io.Closer = (*Store)(nil)
var _ storepkg.Store = (*Store)(nil)
