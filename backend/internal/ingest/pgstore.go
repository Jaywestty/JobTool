package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStore is the Postgres implementation of Store.
type PgStore struct {
	pool *pgxpool.Pool
}

func NewPgStore(pool *pgxpool.Pool) *PgStore { return &PgStore{pool: pool} }

func (s *PgStore) StartRun(ctx context.Context, source string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO source_runs (source, status) VALUES ($1, 'running') RETURNING id`,
		source).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("start source run: %w", err)
	}
	return id, nil
}

func (s *PgStore) FinishRun(ctx context.Context, runID int64, sum RunSummary) error {
	status := "ok"
	if sum.Error != "" {
		status = "partial"
		if sum.Fetched == 0 {
			status = "failed"
		}
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE source_runs
		SET finished_at = now(), status = $2, fetched = $3, kept = $4,
		    inserted = $5, updated = $6, skipped = $7, error = NULLIF($8, '')
		WHERE id = $1`,
		runID, status, sum.Fetched, sum.Kept, sum.Inserted, sum.Updated,
		max0(sum.Fetched-sum.Kept), sum.Error)
	if err != nil {
		return fmt.Errorf("finish source run: %w", err)
	}
	return nil
}

// UpsertJobs inserts or refreshes rows in one statement. url_hash is unique, so
// a conflicting row is refreshed rather than duplicated. A content_hash match
// from another source is also treated as a duplicate.
func (s *PgStore) UpsertJobs(ctx context.Context, records []JobRecord) (int, int, error) {
	if len(records) == 0 {
		return 0, 0, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	inserted, updated := 0, 0
	for _, r := range records {
		var isNew bool
		err := tx.QueryRow(ctx, `
			INSERT INTO jobs (
				canonical_url, url_hash, content_hash, source, company, title,
				description, location, remote_scope, seniority, role_family,
				salary_min, salary_max, salary_currency, posted_at,
				first_seen_at, last_seen_at, is_active
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, $8, $9, $10, $11,
				$12, $13, $14, $15,
				$16, now(), true
			)
			ON CONFLICT (url_hash) DO UPDATE SET
				last_seen_at = now(),
				is_active = true,
				description = EXCLUDED.description,
				title = EXCLUDED.title,
				remote_scope = EXCLUDED.remote_scope,
				seniority = EXCLUDED.seniority,
				role_family = EXCLUDED.role_family,
				posted_at = COALESCE(EXCLUDED.posted_at, jobs.posted_at)
			RETURNING (xmax = 0)`,
			r.CanonicalURL, r.URLHash, r.ContentHash, r.Source, r.Company, r.Title,
			r.Description, r.Location, r.Scope(), r.Seniority, r.RoleFamily,
			r.SalaryMin, r.SalaryMax, nullIfEmpty(r.SalaryCurrency), r.PostedAt,
			r.FirstSeenAt).Scan(&isNew)
		if err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return inserted, updated, fmt.Errorf("upsert job: %w", err)
		}
		if isNew {
			inserted++
		} else {
			updated++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return inserted, updated, err
	}
	return inserted, updated, nil
}

func (s *PgStore) LastRunAt(ctx context.Context, source string) (time.Time, error) {
	var t time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT started_at FROM source_runs WHERE source = $1 ORDER BY started_at DESC LIMIT 1`,
		source).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// TryLockRun uses a session-scoped advisory lock keyed by the source name.
func (s *PgStore) TryLockRun(ctx context.Context, source string) (bool, func(), error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, func() {}, err
	}
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, "ingest:"+source).Scan(&got); err != nil {
		conn.Release()
		return false, func() {}, err
	}
	if !got {
		conn.Release()
		return false, func() {}, nil
	}
	release := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock(hashtext($1))`, "ingest:"+source)
		conn.Release()
	}
	return true, release, nil
}

// Scope returns the stored remote_scope, defaulting to unknown for remote rows.
func (r JobRecord) Scope() string {
	if r.RemoteScope != "" {
		return r.RemoteScope
	}
	if r.IsRemote {
		return ScopeUnknown
	}
	return ScopeUnknown
}

func isUniqueViolation(err error) bool {
	// pgx returns a *pgconn.PgError with Code 23505.
	type pgErr interface{ SQLState() string }
	var pe pgErr
	if errors.As(err, &pe) {
		return pe.SQLState() == "23505"
	}
	return false
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
