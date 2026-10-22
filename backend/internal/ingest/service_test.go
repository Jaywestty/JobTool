package ingest

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeStore struct {
	runs     []RunSummary
	inserted int
	updated  int
	last     map[string]time.Time
	seen     map[string]bool
	now      time.Time
}

func newFakeStore() *fakeStore {
	return &fakeStore{last: map[string]time.Time{}, seen: map[string]bool{}}
}

func (f *fakeStore) StartRun(ctx context.Context, source string) (int64, error) {
	f.last[source] = f.now
	return 1, nil
}

func (f *fakeStore) FinishRun(ctx context.Context, runID int64, sum RunSummary) error {
	f.runs = append(f.runs, sum)
	return nil
}

func (f *fakeStore) UpsertJobs(ctx context.Context, records []JobRecord) (int, int, error) {
	ins, upd := 0, 0
	for _, r := range records {
		key := string(r.URLHash)
		if f.seen[key] {
			upd++
			continue
		}
		f.seen[key] = true
		ins++
	}
	f.inserted += ins
	f.updated += upd
	return ins, upd, nil
}

func (f *fakeStore) LastRunAt(ctx context.Context, source string) (time.Time, error) {
	return f.last[source], nil
}

func (f *fakeStore) TryLockRun(ctx context.Context, source string) (bool, func(), error) {
	return true, func() {}, nil
}

type staticSource struct {
	name  string
	jobs  []RawJob
	iters int
}

func (s *staticSource) Name() string         { return s.name }
func (s *staticSource) Interval() int        { return 60 }
func (s *staticSource) RequiresRobots() bool { return false }
func (s *staticSource) Fetch(ctx context.Context) ([]RawJob, error) {
	s.iters++
	return s.jobs, nil
}

func TestServiceSecondRunIsIdempotent(t *testing.T) {
	cfg := testConfig()
	src := &staticSource{
		name: "test",
		jobs: []RawJob{
			{Title: "Backend Engineer", Company: "Acme", Description: "Build services in Go.", URL: "https://example.com/1", IsRemote: ptrBool(true)},
			{Title: "Data Engineer", Company: "Acme", Description: "Build pipelines.", URL: "https://example.com/2", IsRemote: ptrBool(true)},
			{Title: "Sales Manager", Company: "Acme", Description: "Sell.", URL: "https://example.com/3"},
		},
	}
	reg := &Registry{sources: map[string]Source{"test": src}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	store := newFakeStore()
	store.now = time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	svc := NewService(cfg, reg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	first, err := svc.RunOne(context.Background(), "test")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Fetched != 3 || first.Kept != 2 || first.Inserted != 2 || first.Updated != 0 {
		t.Fatalf("first run summary = %+v", first)
	}

	second, err := svc.RunOne(context.Background(), "test")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 2 {
		t.Fatalf("second run should insert nothing, got %+v", second)
	}
	if len(store.runs) != 2 {
		t.Fatalf("expected 2 run records, got %d", len(store.runs))
	}
}

func TestServiceUnknownSource(t *testing.T) {
	cfg := testConfig()
	reg := &Registry{sources: map[string]Source{}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	svc := NewService(cfg, reg, newFakeStore(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := svc.RunOne(context.Background(), "nope")
	if !IsUnknownSource(err) {
		t.Fatalf("expected unknown source, got %v", err)
	}
}

func TestRunAllSinceRespectsInterval(t *testing.T) {
	cfg := testConfig()
	src := &staticSource{name: "test", jobs: []RawJob{{Title: "Backend Engineer", Company: "A", Description: "Go team work and build services.", URL: "https://example.com/1", IsRemote: ptrBool(true)}}}
	reg := &Registry{sources: map[string]Source{"test": src}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	store := newFakeStore()
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	store.now = now
	svc := NewService(cfg, reg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.now = func() time.Time { return now }

	svc.RunAllSince(context.Background())
	if src.iters != 1 {
		t.Fatalf("first tick iters = %d", src.iters)
	}
	svc.RunAllSince(context.Background())
	if src.iters != 1 {
		t.Fatalf("second tick should be within interval, iters = %d", src.iters)
	}
	svc.now = func() time.Time { return now.Add(2 * time.Minute) }
	svc.RunAllSince(context.Background())
	if src.iters != 2 {
		t.Fatalf("tick after interval iters = %d", src.iters)
	}
}
