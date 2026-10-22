package ingest

import (
	"context"
	"time"
)

// JobRecord is one normalized, fingerprinted job ready to insert.
type JobRecord struct {
	Job
	Fingerprints
	FirstSeenAt time.Time
}

// RunSummary is what an ingest run reports.
type RunSummary struct {
	Source   string        `json:"source"`
	Fetched  int           `json:"fetched"`
	Kept     int           `json:"kept"`
	Inserted int           `json:"inserted"`
	Updated  int           `json:"updated"`
	Duration time.Duration `json:"duration_ms"`
	Error    string        `json:"error,omitempty"`
}

// Store persists ingest output and run records.
type Store interface {
	StartRun(ctx context.Context, source string) (int64, error)
	FinishRun(ctx context.Context, runID int64, summary RunSummary) error
	// UpsertJobs inserts new rows and refreshes seen timestamps on existing
	// ones. It returns inserted and updated counts.
	UpsertJobs(ctx context.Context, records []JobRecord) (inserted, updated int, err error)
	// LastRunAt returns the most recent run start for a source, zero when
	// none exists.
	LastRunAt(ctx context.Context, source string) (time.Time, error)
	// TryLockRun takes a per-source advisory lock so the manual trigger and the
	// scheduler cannot ingest the same source at once. release is always safe
	// to call.
	TryLockRun(ctx context.Context, source string) (got bool, release func(), err error)
}
