package ingest

import "context"

// Source is one job feed. Implementations return normalized rows only; the
// pipeline applies config-driven classification, dedupe and persistence.
type Source interface {
	// Name is the stable source identifier stored on jobs and source_runs.
	Name() string
	// Interval is how often the scheduler should run this source.
	Interval() int
	// RequiresRobots is true for crawler-style sources that must honor
	// robots.txt.
	RequiresRobots() bool
	// Fetch returns every posting the source currently exposes.
	Fetch(ctx context.Context) ([]RawJob, error)
}

// RawJob is a connector's output before normalization. Connectors fill the
// native fields; Normalize applies the timestamp, HTML, salary and scope rules.
type RawJob struct {
	Title           string
	Company         string
	Description     string
	Location        string
	URL             string
	PostedAt        string
	PostedEpoch     *float64
	IsRemote        *bool
	SalaryMin       *float64
	SalaryMax       *float64
	SalaryCurrency  string
	NativeSeniority string
	ExternalID      string
}

// validatable is implemented by connectors with config slugs that must be
// probed at startup.
type validatable interface {
	Validate(ctx context.Context) error
}
