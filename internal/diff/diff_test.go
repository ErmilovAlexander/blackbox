package diff

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
)

func TestBetweenReturnsStableJSONPointerChanges(t *testing.T) {
	previous := json.RawMessage(`{
		"metadata":{"labels":{"app":"api","old":"remove-me"}},
		"spec":{"ports":[80,443],"replicas":2}
	}`)
	current := json.RawMessage(`{
		"metadata":{"labels":{"app":"api","new":"added"}},
		"spec":{"ports":[8080],"replicas":3}
	}`)

	changes, err := Between(previous, current)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{Operation: OperationAdd, Path: "/metadata/labels/new", After: json.RawMessage(`"added"`)},
		{Operation: OperationRemove, Path: "/metadata/labels/old", Before: json.RawMessage(`"remove-me"`)},
		{Operation: OperationReplace, Path: "/spec/ports", Before: json.RawMessage(`[80,443]`), After: json.RawMessage(`[8080]`)},
		{Operation: OperationReplace, Path: "/spec/replicas", Before: json.RawMessage(`2`), After: json.RawMessage(`3`)},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("Between() = %#v, want %#v", changes, want)
	}
}

func TestBetweenEscapesJSONPointerTokens(t *testing.T) {
	changes, err := Between(json.RawMessage(`{}`), json.RawMessage(`{"a/b~c":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != "/a~1b~0c" {
		t.Fatalf("unexpected changes: %#v", changes)
	}
}

func TestBetweenDistinguishesMissingFromNull(t *testing.T) {
	added, err := Between(nil, json.RawMessage(`null`))
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].Operation != OperationAdd || string(added[0].After) != "null" {
		t.Fatalf("unexpected ADD: %#v", added)
	}

	removed, err := Between(json.RawMessage(`null`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].Operation != OperationRemove || string(removed[0].Before) != "null" {
		t.Fatalf("unexpected REMOVE: %#v", removed)
	}
}

func TestBuildSkipsSnapshotsAndNoOpUpdates(t *testing.T) {
	now := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC)
	records := []model.Record{
		{ID: "snapshot", ObservedAt: now, Action: model.ActionSnapshot, Object: json.RawMessage(`{"a":1}`)},
		{ID: "no-op", ObservedAt: now.Add(time.Second), Action: model.ActionUpdate, Previous: json.RawMessage(`{"a":1}`), Object: json.RawMessage(`{"a":1}`)},
		{ID: "changed", ObservedAt: now.Add(2 * time.Second), Action: model.ActionUpdate, Kind: "Pod", Name: "api", Previous: json.RawMessage(`{"a":1}`), Object: json.RawMessage(`{"a":2}`)},
	}

	entries, err := Build(records)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].RecordID != "changed" || entries[0].Changes[0].Path != "/a" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestBetweenRejectsInvalidJSON(t *testing.T) {
	if _, err := Between(json.RawMessage(`{"a":`), json.RawMessage(`{}`)); err == nil {
		t.Fatal("Between() accepted invalid JSON")
	}
}
