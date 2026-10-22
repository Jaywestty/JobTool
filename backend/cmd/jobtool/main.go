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
	handler := platform.NewHTTPHandler(log, cfg.WebOrigin, checks)
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

func runWorker(ctx context.Context, log *slog.Logger, cfg *platform.Config, pool *pgxpool.Pool) error {
	q := jobs.NewQueue(pool)
	drain := func(ctx context.Context) error {
		tasks, err := q.Claim(ctx, 20, 10*time.Minute)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			log.Info("task claimed", "id", t.ID, "kind", t.Kind, "attempts", t.Attempts)
			if err := q.Complete(ctx, t.ID); err != nil {
				return err
			}
		}
		return nil
	}
	return platform.Every(ctx, 5*time.Second, drain)
}

func runScheduler(ctx context.Context, log *slog.Logger, cfg *platform.Config, pool *pgxpool.Pool) error {
	q := jobs.NewQueue(pool)
	tick := func(ctx context.Context) error {
		if err := q.Enqueue(ctx, jobs.TaskIngest, "ingest:tick", nil, time.Now()); err != nil {
			log.Warn("enqueue ingest failed", "error", err)
		}
		return nil
	}
	return platform.Every(ctx, time.Minute, tick)
}


