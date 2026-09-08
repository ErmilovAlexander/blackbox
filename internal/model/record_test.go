package model

import (
	"testing"
	"time"
)

func TestRecordValidate(t *testing.T) {
	valid := Record{
		SchemaVersion: SchemaVersionV1Alpha1,
		ID:            "record-1",
		ObservedAt:    time.Now().UTC(),
		Source:        SourceKubernetes,
		Action:        ActionSnapshot,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{"schema", func(r *Record) { r.SchemaVersion = "kbb.io/record/v2" }},
		{"id", func(r *Record) { r.ID = "" }},
		{"time", func(r *Record) { r.ObservedAt = time.Time{} }},
		{"source", func(r *Record) { r.Source = Source("unknown") }},
		{"action", func(r *Record) { r.Action = Action("unknown") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := valid
			tt.mutate(&record)
			if err := record.Validate(); err == nil {
				t.Fatal("Validate() returned nil for an invalid record")
			}
		})
	}
}
