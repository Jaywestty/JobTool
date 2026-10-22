package ingest

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Service runs sources and persists their output.
type Service struct {
	registry *Registry
	store    Store
	norm     *normalizer
	cfg      *Config
	log      *slog.Logger
	now      func() time.Time
}

// NewService wires the pipeline.
func NewService(cfg *Config, registry *Registry, store Store, log *slog.Logger) *Service {
	return &Service{
		registry: registry,
		store:    store,
		norm:     newNormalizer(cfg, time.Now),
		cfg:      cfg,
		log:      log,
		now:      time.Now,
	}
}

// RunOne ingests a single source and records a source_runs row.
func (s *Service) RunOne(ctx context.Context, name string) (RunSummary, error) {
	src, ok := s.registry.Get(name)
	if !ok {
		return RunSummary{}, &UnknownSourceError{Name: name}
	}
	return s.run(ctx, src)
}

// RunAll ingests every registered source sequentially. Each source's failure
// is recorded and does not stop the rest.
func (s *Service) RunAll(ctx context.Context) []RunSummary {
	var out []RunSummary
	for _, src := range s.registry.All() {
		sum, err := s.run(ctx, src)
		if err != nil && !IsAlreadyRunning(err) {
			s.log.Error("ingest source failed", "source", src.Name(), "error", err)
		}
		out = append(out, sum)
	}
	return out
}

// RunAllSince runs sources whose last run is older than their interval. Used
// by the scheduler tick.
func (s *Service) RunAllSince(ctx context.Context) []RunSummary {
	var out []RunSummary
	for _, src := range s.registry.All() {
		last, err := s.store.LastRunAt(ctx, src.Name())
		if err != nil {
			s.log.Warn("could not read last run", "source", src.Name(), "error", err)
			continue
		}
		if !last.IsZero() && s.now().Sub(last) < Interval(src.Interval()) {
			continue
		}
		sum, err := s.run(ctx, src)
		if err != nil && !IsAlreadyRunning(err) {
			s.log.Error("scheduled ingest failed", "source", src.Name(), "error", err)
		}
		out = append(out, sum)
	}
	return out
}

func (s *Service) run(ctx context.Context, src Source) (RunSummary, error) {
	started := s.now()
	sum := RunSummary{Source: src.Name()}

	got, release, err := s.store.TryLockRun(ctx, src.Name())
	if err != nil {
		return sum, err
	}
	if !got {
		s.log.Info("ingest already running for source, skipping", "source", src.Name())
		return sum, errAlreadyRunning
	}
	defer release()

	runID, err := s.store.StartRun(ctx, src.Name())
	if err != nil {
		return sum, err
	}

	raw, fetchErr := src.Fetch(ctx)
	sum.Fetched = len(raw)
	if fetchErr != nil {
		sum.Error = fetchErr.Error()
		s.finish(ctx, runID, sum, started)
		return sum, fetchErr
	}

	seenURL := map[string]bool{}
	seenContent := map[string]bool{}
	records := make([]JobRecord, 0, len(raw))
	for _, r := range raw {
		job, ok := s.norm.Normalize(src.Name(), r)
		if !ok {
			continue
		}
		sum.Kept++
		fp, err := Fingerprint(job)
		if err != nil {
			continue
		}
		urlKey := string(fp.URLHash)
		contentKey := string(fp.ContentHash)
		if seenURL[urlKey] || seenContent[contentKey] {
			continue
		}
		seenURL[urlKey] = true
		seenContent[contentKey] = true
		records = append(records, JobRecord{Job: job, Fingerprints: fp, FirstSeenAt: started})
	}

	inserted, updated, err := s.store.UpsertJobs(ctx, records)
	sum.Inserted = inserted
	sum.Updated = updated
	if err != nil {
		sum.Error = err.Error()
		s.finish(ctx, runID, sum, started)
		return sum, err
	}
	s.finish(ctx, runID, sum, started)

	s.log.Info("ingest source done",
		"source", sum.Source,
		"fetched", sum.Fetched,
		"kept", sum.Kept,
		"inserted", sum.Inserted,
		"updated", sum.Updated,
		"duration_ms", sum.Duration.Milliseconds(),
	)
	return sum, nil
}

func (s *Service) finish(ctx context.Context, runID int64, sum RunSummary, started time.Time) {
	sum.Duration = s.now().Sub(started)
	if err := s.store.FinishRun(ctx, runID, sum); err != nil {
		s.log.Error("could not finish source run", "source", sum.Source, "error", err)
	}
}

// UnknownSourceError is returned for a name not in the registry.
type UnknownSourceError struct{ Name string }

func (e *UnknownSourceError) Error() string { return "unknown source: " + e.Name }

// errAlreadyRunning means another process holds the source lock.
var errAlreadyRunning = errors.New("ingest already running for this source")

// IsAlreadyRunning reports whether err means a concurrent run holds the lock.
func IsAlreadyRunning(err error) bool { return errors.Is(err, errAlreadyRunning) }

// IsUnknownSource reports whether err is an UnknownSourceError.
func IsUnknownSource(err error) bool {
	var u *UnknownSourceError
	return errors.As(err, &u)
}
