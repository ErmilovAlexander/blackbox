// Package diff calculates deterministic structural changes between two
// canonical Kubernetes object snapshots.
package diff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
)

type Operation string

const (
	OperationAdd     Operation = "ADD"
	OperationRemove  Operation = "REMOVE"
	OperationReplace Operation = "REPLACE"
)

// Change identifies one changed JSON value. Path follows RFC 6901 JSON Pointer.
// Arrays are intentionally treated as atomic values because Kubernetes lists
// can be reordered and index-by-index output would imply unreliable identity.
type Change struct {
	Operation Operation       `json:"operation"`
	Path      string          `json:"path"`
	Before    json.RawMessage `json:"before,omitempty"`
	After     json.RawMessage `json:"after,omitempty"`
}

type Entry struct {
	At        time.Time    `json:"at"`
	Kind      string       `json:"kind"`
	Namespace string       `json:"namespace,omitempty"`
	Name      string       `json:"name,omitempty"`
	UID       string       `json:"uid,omitempty"`
	Action    model.Action `json:"action"`
	RecordID  string       `json:"recordId"`
	Changes   []Change     `json:"changes"`
}

// Between returns a stable, path-sorted structural diff. An empty raw message
// means that the object side does not exist (ADD or DELETE), while JSON null is
// treated as an existing JSON value.
func Between(previous, current json.RawMessage) ([]Change, error) {
	before, beforeExists, err := decode(previous)
	if err != nil {
		return nil, fmt.Errorf("decode previous object: %w", err)
	}
	after, afterExists, err := decode(current)
	if err != nil {
		return nil, fmt.Errorf("decode current object: %w", err)
	}

	changes := make([]Change, 0, 8)
	if err := walk("", before, beforeExists, after, afterExists, &changes); err != nil {
		return nil, err
	}
	return changes, nil
}

// Build creates user-facing diff entries and omits initial snapshots and
// no-op updates. Snapshot records describe the starting state, not a change.
func Build(records []model.Record) ([]Entry, error) {
	entries := make([]Entry, 0, len(records))
	for _, record := range records {
		if record.Action == model.ActionSnapshot || record.Action == model.ActionSignal {
			continue
		}
		changes, err := Between(record.Previous, record.Object)
		if err != nil {
			return nil, fmt.Errorf("diff record %s: %w", record.ID, err)
		}
		if len(changes) == 0 {
			continue
		}
		entries = append(entries, Entry{
			At:        record.ObservedAt,
			Kind:      record.Kind,
			Namespace: record.Namespace,
			Name:      record.Name,
			UID:       record.UID,
			Action:    record.Action,
			RecordID:  record.ID,
			Changes:   changes,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].At.Before(entries[j].At) })
	return entries, nil
}

func decode(raw json.RawMessage) (any, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, false, fmt.Errorf("multiple JSON values")
		}
		return nil, false, err
	}
	return value, true, nil
}

func walk(path string, before any, beforeExists bool, after any, afterExists bool, changes *[]Change) error {
	switch {
	case !beforeExists && !afterExists:
		return nil
	case !beforeExists:
		raw, err := marshal(after)
		if err != nil {
			return err
		}
		*changes = append(*changes, Change{Operation: OperationAdd, Path: path, After: raw})
		return nil
	case !afterExists:
		raw, err := marshal(before)
		if err != nil {
			return err
		}
		*changes = append(*changes, Change{Operation: OperationRemove, Path: path, Before: raw})
		return nil
	}

	beforeMap, beforeIsMap := before.(map[string]any)
	afterMap, afterIsMap := after.(map[string]any)
	if beforeIsMap && afterIsMap {
		keys := make([]string, 0, len(beforeMap)+len(afterMap))
		seen := make(map[string]struct{}, len(beforeMap)+len(afterMap))
		for key := range beforeMap {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		for key := range afterMap {
			if _, ok := seen[key]; !ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			beforeValue, hasBefore := beforeMap[key]
			afterValue, hasAfter := afterMap[key]
			if err := walk(join(path, key), beforeValue, hasBefore, afterValue, hasAfter, changes); err != nil {
				return err
			}
		}
		return nil
	}

	if reflect.DeepEqual(before, after) {
		return nil
	}
	beforeRaw, err := marshal(before)
	if err != nil {
		return err
	}
	afterRaw, err := marshal(after)
	if err != nil {
		return err
	}
	*changes = append(*changes, Change{Operation: OperationReplace, Path: path, Before: beforeRaw, After: afterRaw})
	return nil
}

func marshal(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode diff value: %w", err)
	}
	return raw, nil
}

func join(parent, token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return parent + "/" + token
}
