package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobtool/jobtool/backend/internal/ingest"
	"github.com/jobtool/jobtool/backend/internal/jobs"
	"github.com/jobtool/jobtool/backend/internal/platform"
	"github.com/jobtool/jobtool/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	roleFlag := flag.String("role", "", "process role: api, worker or scheduler")
	migrateFlag := flag.Bool("migrate", false, "apply migrations and exit")
	flag.Parse()

	cfg, err := platform.LoadWithRole(os.Getenv, *roleFlag)
	if err != nil {
		return err
	}
	if *migrateFlag {
		cfg.Migrations = true
	}
	if !validRole(cfg.Role) && !cfg.Migrations {
		return fmt.Errorf("JOBTOOL_ROLE must be api, worker or scheduler, got %q", cfg.Role)
	}

	log := platform.NewLogger(cfg.LogLevel)
	log.Info("starting jobtool", "role", cfg.Role, "env", cfg.AppEnv, "migrate", cfg.Migrations)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Migrations {
		log.Info("applying migrations")
		if err := store.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
		log.Info("migrations complete")
		if *migrateFlag && *roleFlag == "" {
			return nil
		}
	}

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch cfg.Role {
	case "api":
		return runAPI(ctx, log, cfg, pool)
	case "worker":
		return runWorker(ctx, log, cfg, pool)
	case "scheduler":
		return runScheduler(ctx, log, cfg, pool)
	default:
		return fmt.Errorf("unknown role %q", cfg.Role)
	}
}

func validRole(role string) bool {
	return role == "api" || role == "worker" || role == "scheduler"
}

func runAPI(ctx context.Context, log *slog.Logger, cfg *platform.Config, pool *pgxpool.Pool) error {
	checks := map[string]platform.HealthChecker{
		"postgres": func(ctx context.Context) error { return pool.Ping(ctx) },
	}
	bundle, err := newIngest(ctx, log, cfg, pool)
	if err != nil {
		return err
	}
	routes := []platform.Route{
		{Pattern: "GET /health", Handler: platform.HealthHandler(log, checks)},
		{Pattern: "GET /api/health", Handler: platform.HealthHandler(log, checks)},
		{Pattern: "GET /api/ingest/sources", Handler: triggerHandler(bundle, cfg.Ingest.TriggerToken, false)},
		{Pattern: "POST /api/ingest/run", Handler: triggerHandler(bundle, cfg.Ingest.TriggerToken, true)},
	}
	handler := platform.NewHTTPHandler(log, cfg.WebOrigin, routes)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// runWorker drains ingest tasks from the Postgres queue and runs them.
func runWorker(ctx context.Context, log *slog.Logger, cfg *platform.Config, pool *pgxpool.Pool) error {
	bundle, err := newIngest(ctx, log, cfg, pool)
	if err != nil {
		return err
	}
	q := jobs.NewQueue(pool)
	drain := func(ctx context.Context) error {
		tasks, err := q.Claim(ctx, 20, 10*time.Minute)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			jobCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
			err := runIngestTask(jobCtx, log, bundle, t)
			cancel()
			if err != nil {
				log.Error("ingest task failed", "id", t.ID, "kind", t.Kind, "error", err)
				if ferr := q.Fail(ctx, t.ID, t.Attempts, err.Error()); ferr != nil {
					log.Error("could not record task failure", "id", t.ID, "error", ferr)
				}
				continue
			}
			if err := q.Complete(ctx, t.ID); err != nil {
				return err
			}
		}
		return nil
	}
	return platform.Every(ctx, 5*time.Second, drain)
}

// runScheduler enqueues an ingest task for each source whose interval has
// elapsed, every minute.
func runScheduler(ctx context.Context, log *slog.Logger, cfg *platform.Config, pool *pgxpool.Pool) error {
	bundle, err := newIngest(ctx, log, cfg, pool)
	if err != nil {
		return err
	}
	q := jobs.NewQueue(pool)
	store := ingest.NewPgStore(pool)
	tick := func(ctx context.Context) error {
		for _, src := range bundle.registry.All() {
			last, err := store.LastRunAt(ctx, src.Name())
			if err != nil {
				log.Warn("could not read last run", "source", src.Name(), "error", err)
				continue
			}
			if !last.IsZero() && time.Since(last) < ingest.Interval(src.Interval()) {
				continue
			}
			payload := []byte(`{"source":"` + src.Name() + `"}`)
			if err := q.Enqueue(ctx, jobs.TaskIngestSource, src.Name(), payload, time.Now()); err != nil {
				log.Warn("enqueue ingest failed", "source", src.Name(), "error", err)
			}
		}
		return nil
	}
	return platform.Every(ctx, time.Minute, tick)
}
