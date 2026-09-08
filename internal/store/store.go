package store

import (
	"context"
	"time"

	"github.com/ErmilovAlexander/blackbox/internal/model"
)

type Query struct {
	From      time.Time
	To        time.Time
	Namespace string
	Kind      string
	Name      string
	UID       string
	Limit     int
}

type Store interface {
	Append(context.Context, model.Record) error
	Query(context.Context, Query) ([]model.Record, error)
	Close() error
}
