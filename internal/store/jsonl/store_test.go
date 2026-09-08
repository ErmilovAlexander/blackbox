package jsonl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
	storepkg "github.com/ErmilovAlexander/blackbox/internal/store"
)

func TestAppendAndQuery(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, time.Hour, 1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Second)
	for i, name := range []string{"a", "b"} {
		r := model.Record{SchemaVersion: model.SchemaVersionV1Alpha1, ID: name, ObservedAt: now.Add(time.Duration(i) * time.Second), Source: model.SourceKubernetes, Action: model.ActionUpdate, Kind: "Pod", Namespace: "demo", Name: name}
		if err := s.Append(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Query(context.Background(), storepkg.Query{Namespace: "demo", Kind: "pod", Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("unexpected query result: %#v", got)
	}
}

func TestOpenReadOnlyDoesNotCreateSegment(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "segment-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("read-only open created segments: %v", files)
	}

	r := model.Record{SchemaVersion: model.SchemaVersionV1Alpha1, ID: "x", ObservedAt: time.Now(), Source: model.SourceKubernetes, Action: model.ActionAdd}
	if err := s.Append(context.Background(), r); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Append() error = %v, want %v", err, ErrReadOnly)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Query(context.Background(), storepkg.Query{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Query() after Close error = %v, want %v", err, ErrClosed)
	}
}

func TestOpenRejectsSegmentLargerThanBoundedStore(t *testing.T) {
	_, err := Open(t.TempDir(), time.Hour, 1024, 2048)
	if err == nil {
		t.Fatal("Open() accepted maxSegmentBytes greater than maxStoreBytes")
	}
}

func TestOpenReadOnlyRequiresExistingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	_, err := OpenReadOnly(dir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenReadOnly() error = %v, want os.ErrNotExist", err)
	}
}
