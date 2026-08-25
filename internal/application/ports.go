package application

import (
	"context"
	"time"

	"heritage-tree-relocation-permit/internal/domain"
)

type TimelineEntry struct {
	Sequence       int64     `json:"sequence"`
	EventType      string    `json:"eventType"`
	CaseID         string    `json:"caseId"`
	CaseVersion    int64     `json:"caseVersion"`
	IdempotencyKey string    `json:"idempotencyKey,omitempty"`
	OccurredAt     time.Time `json:"occurredAt"`
	Summary        string    `json:"summary"`
}

type CommitRequest struct {
	CaseID          string
	ExpectedVersion int64
	IdempotencyKey  string
	EventType       string
	Summary         string
	OccurredAt      time.Time
	State           domain.RelocationCase
}

type CommitResult struct {
	State      domain.RelocationCase
	Sequence   int64
	Idempotent bool
}

type Repository interface {
	Create(context.Context, domain.RelocationCase, string, time.Time) (CommitResult, error)
	Get(context.Context, string) (domain.RelocationCase, error)
	List(context.Context) ([]domain.RelocationCase, error)
	Commit(context.Context, CommitRequest) (CommitResult, error)
	Timeline(context.Context, string) ([]TimelineEntry, error)
	NextCaseNumber(context.Context, time.Time) (string, error)
	NextPermitNumber(context.Context, time.Time) (string, error)
	Close() error
}

type Clock interface{ Now() time.Time }

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type IDGenerator interface{ NewID(prefix string) string }
