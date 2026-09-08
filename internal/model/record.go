package model

import (
	"encoding/json"
	"fmt"
	"time"
)

type Action string

const (
	SchemaVersionV1Alpha1 = "kbb.io/record/v1alpha1"

	ActionSnapshot Action = "SNAPSHOT"
	ActionAdd      Action = "ADD"
	ActionUpdate   Action = "UPDATE"
	ActionDelete   Action = "DELETE"
	ActionSignal   Action = "SIGNAL"
)

type Source string

const (
	SourceKubernetes Source = "kubernetes-api"
	SourceNode       Source = "node-agent"
	SourceRule       Source = "rule-engine"
)

// Record is the canonical, AI-independent event format used by the recorder.
// The schema is intentionally generic so future analyzers can consume exported
// bundles without being linked into the core process.
type Record struct {
	SchemaVersion string            `json:"schemaVersion"`
	ID            string            `json:"id"`
	ObservedAt    time.Time         `json:"observedAt"`
	Source        Source            `json:"source"`
	Action        Action            `json:"action"`
	Cluster       string            `json:"cluster,omitempty"`
	APIVersion    string            `json:"apiVersion,omitempty"`
	Kind          string            `json:"kind,omitempty"`
	Namespace     string            `json:"namespace,omitempty"`
	Name          string            `json:"name,omitempty"`
	UID           string            `json:"uid,omitempty"`
	ResourceVer   string            `json:"resourceVersion,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	Summary       string            `json:"summary,omitempty"`
	Object        json.RawMessage   `json:"object,omitempty"`
	Previous      json.RawMessage   `json:"previous,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

func (r Record) Validate() error {
	if r.SchemaVersion != SchemaVersionV1Alpha1 {
		return fmt.Errorf("unsupported schemaVersion %q", r.SchemaVersion)
	}
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("observedAt is required")
	}
	if r.Source == "" {
		return fmt.Errorf("source is required")
	}
	switch r.Source {
	case SourceKubernetes, SourceNode, SourceRule:
	default:
		return fmt.Errorf("unsupported source %q", r.Source)
	}
	switch r.Action {
	case ActionSnapshot, ActionAdd, ActionUpdate, ActionDelete, ActionSignal:
	default:
		return fmt.Errorf("unsupported action %q", r.Action)
	}
	return nil
}
