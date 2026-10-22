package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobtool/jobtool/backend/internal/ingest"
	"github.com/jobtool/jobtool/backend/internal/platform"
)

// ingestBundle is the wired ingestion pipeline shared by worker, scheduler and
// the API trigger endpoint.
type ingestBundle struct {
	service  *ingest.Service
	registry *ingest.Registry
	client   *ingest.Client
	cfg      *ingest.Config
}

// newIngest loads the YAML config, builds the polite client and the registry,
// and probes ATS slugs once.
func newIngest(ctx context.Context, log *slog.Logger, cfg *platform.Config, pool *pgxpool.Pool) (*ingestBundle, error) {
	ic, err := ingest.LoadConfig(ingestConfigPath(cfg))
	if err != nil {
		return nil, err
	}
	client := ingest.NewClient(ingest.ClientOptions{
		UserAgent: cfg.Sources.UserAgent,
		Timeout:   time.Duration(cfg.Sources.TimeoutS) * time.Second,
		MinGap:    time.Duration(cfg.Ingest.MinHostGapS) * time.Second,
		Retries:   cfg.Ingest.Retries,
		Backoff:   time.Duration(cfg.Ingest.BackoffS) * time.Second,
	})
	registry := ingest.BuildRegistry(ic, client, log)
	registry.ValidateSlugs(ctx)
	log.Info("ingest registry ready", "sources", len(registry.Names()), "names", registry.Names())
	svc := ingest.NewService(ic, registry, ingest.NewPgStore(pool), log)
	return &ingestBundle{service: svc, registry: registry, client: client, cfg: ic}, nil
}

func ingestConfigPath(cfg *platform.Config) string {
	if cfg.Ingest.ConfigPath != "" {
		return cfg.Ingest.ConfigPath
	}
	return ingest.DefaultConfigPath
}
