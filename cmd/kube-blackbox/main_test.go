package main

import (
	"context"
	"testing"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
	jsonlstore "github.com/ErmilovAlexander/blackbox/internal/store/jsonl"
)

func TestQueryOptionsLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := jsonlstore.Open(dir, time.Hour, 1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC)
	for _, record := range []model.Record{
		{SchemaVersion: model.SchemaVersionV1Alpha1, ID: "pod", ObservedAt: now, Source: model.SourceKubernetes, Action: model.ActionAdd, Kind: "Pod", Namespace: "demo", Name: "api", UID: "pod-uid"},
		{SchemaVersion: model.SchemaVersionV1Alpha1, ID: "service", ObservedAt: now, Source: model.SourceKubernetes, Action: model.ActionAdd, Kind: "Service", Namespace: "demo", Name: "api", UID: "service-uid"},
	} {
		if err := store.Append(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	records, err := (queryOptions{dataDir: dir, ns: "demo", kind: "pod", uid: "pod-uid", limit: 10}).load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "pod" {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestQueryOptionsRejectsInvalidRangeAndLimit(t *testing.T) {
	if _, err := (queryOptions{fromText: "2026-09-08T10:00:00Z", toText: "2026-09-08T09:00:00Z"}).load(); err == nil {
		t.Fatal("load() accepted an inverted time range")
	}
	if _, err := (queryOptions{limit: -1}).load(); err == nil {
		t.Fatal("load() accepted a negative limit")
	}
}
