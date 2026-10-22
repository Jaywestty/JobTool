package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jobtool/jobtool/backend/internal/ingest"
	"github.com/jobtool/jobtool/backend/internal/jobs"
)

// runIngestTask executes one queue task.
func runIngestTask(ctx context.Context, log *slog.Logger, bundle *ingestBundle, t jobs.Task) error {
	switch t.Kind {
	case jobs.TaskIngestSource:
		var payload struct {
			Source string `json:"source"`
		}
		_ = json.Unmarshal(t.Payload, &payload)
		if payload.Source == "" {
			return fmt.Errorf("ingest task has no source")
		}
		_, err := bundle.service.RunOne(ctx, payload.Source)
		return err
	case jobs.TaskIngest:
		bundle.service.RunAllSince(ctx)
		return nil
	default:
		log.Warn("ignoring unknown task kind", "kind", t.Kind)
		return nil
	}
}

// triggerHandler serves GET /api/ingest/sources and POST /api/ingest/run.
func triggerHandler(bundle *ingestBundle, token string, run bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" && !authorized(r, token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if !run {
			writeJSON(w, http.StatusOK, map[string]any{"sources": bundle.registry.Names()})
			return
		}
		var body struct {
			Source string `json:"source"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)
		if body.Source != "" {
			sum, err := bundle.service.RunOne(r.Context(), body.Source)
			if err != nil {
				status := http.StatusInternalServerError
				if ingest.IsUnknownSource(err) {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]any{"error": err.Error(), "run": sum})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"runs": []ingest.RunSummary{sum}})
			return
		}
		sums := bundle.service.RunAll(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"runs": sums})
	}
}

func authorized(r *http.Request, token string) bool {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") && strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")) == token {
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
