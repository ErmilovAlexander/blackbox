package timeline

import (
	"sort"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
)

type Entry struct {
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	Namespace string    `json:"namespace,omitempty"`
	Name      string    `json:"name,omitempty"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	RecordID  string    `json:"recordId"`
}

func Build(records []model.Record) []Entry {
	out := make([]Entry, 0, len(records))
	for _, r := range records {
		out = append(out, Entry{At: r.ObservedAt, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name, Action: string(r.Action), Reason: r.Reason, Summary: r.Summary, RecordID: r.ID})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}
