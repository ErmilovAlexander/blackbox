package timeline

import (
	"testing"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
)

func TestBuildSortsChronologicallyAndPreservesEvidenceID(t *testing.T) {
	base := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC)
	records := []model.Record{
		{ID: "later", ObservedAt: base.Add(time.Minute), Action: model.ActionUpdate, Kind: "Pod", Name: "api"},
		{ID: "earlier", ObservedAt: base, Action: model.ActionSnapshot, Kind: "Pod", Name: "api"},
	}

	entries := Build(records)
	if len(entries) != 2 {
		t.Fatalf("Build() returned %d entries, want 2", len(entries))
	}
	if entries[0].RecordID != "earlier" || entries[1].RecordID != "later" {
		t.Fatalf("unexpected order: %#v", entries)
	}
	if entries[0].Action != string(model.ActionSnapshot) {
		t.Fatalf("first action = %q, want SNAPSHOT", entries[0].Action)
	}
}
