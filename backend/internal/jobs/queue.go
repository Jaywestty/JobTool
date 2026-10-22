package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Queue is the Postgres-backed job queue. Workers claim rows with
// SELECT ... FOR UPDATE SKIP LOCKED, which needs no broker.
type Queue struct {
	pool *pgxpool.Pool
}

func NewQueue(pool *pgxpool.Pool) *Queue { return &Queue{pool: pool} }

// Task is one claimed queue row.
type Task struct {
	ID       int64
	Kind     string
	Payload  []byte
	Attempts int
}

// Task names are strings so new task kinds need no schema change.
const (
	TaskIngest       = "ingest"
	TaskIngestSource = "ingest_source"
	TaskMatch        = "match_user"
	TaskDigest       = "send_digest"
	TaskEmbed        = "embed_jobs"
)

// Enqueue adds a task. A partial unique index on (kind, dedupe_key) means a
// duplicate enqueue is a no-op.
func (q *Queue) Enqueue(ctx context.Context, kind, dedupeKey string, payload []byte, availableAt time.Time) error {
	if payload == nil {
		payload = []byte("{}")
	}
	_, err := q.pool.Exec(ctx, `
		INSERT INTO task_queue (kind, dedupe_key, payload, available_at)
		VALUES ($1, NULLIF($2, ''), $3, $4)
		ON CONFLICT DO NOTHING`,
		kind, dedupeKey, payload, availableAt)
	if err != nil {
		return fmt.Errorf("enqueue %s: %w", kind, err)
	}
	return nil
}

// Claim leases up to limit due tasks to one worker.
func (q *Queue) Claim(ctx context.Context, limit int, lease time.Duration) ([]Task, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, kind, payload, attempts
		FROM task_queue
		WHERE status = 'pending' AND available_at <= now()
		ORDER BY available_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Kind, &t.Payload, &t.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		tasks = append(tasks, t)
		ids = append(ids, t.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE task_queue
		SET status = 'running', attempts = attempts + 1, locked_until = now() + $2::interval
		WHERE id = ANY($1)`, ids, lease.String()); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Attempts++
	}
	return tasks, nil
}

// Complete marks a task done.
func (q *Queue) Complete(ctx context.Context, id int64) error {
	_, err := q.pool.Exec(ctx, `UPDATE task_queue SET status = 'done', finished_at = now(), locked_until = NULL WHERE id = $1`, id)
	return err
}

// Fail requeues a task with exponential backoff, or marks it failed once
// attempts reach maxAttempts.
func (q *Queue) Fail(ctx context.Context, id int64, attempts int, cause string) error {
	backoff := time.Duration(1<<min(attempts, 6)) * time.Minute
	_, err := q.pool.Exec(ctx, `
		UPDATE task_queue
		SET status = CASE WHEN attempts >= $2 THEN 'failed' ELSE 'pending' END,
		    available_at = now() + $3::interval,
		    locked_until = NULL,
		    last_error = $4
		WHERE id = $1`, id, maxAttempts, backoff.String(), cause)
	return err
}

const maxAttempts = 5
