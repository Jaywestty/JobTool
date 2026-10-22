package platform

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func fullEnv() map[string]string {
	return map[string]string{
		"JOBTOOL_ROLE":                    "api",
		"JOBTOOL_HTTP_ADDR":               ":8080",
		"JOBTOOL_DATABASE_URL":            "postgres://jobtool:jobtool@postgres:5432/jobtool?sslmode=disable",
		"JOBTOOL_LOG_LEVEL":               "info",
		"JOBTOOL_WEB_ORIGIN":              "http://localhost:3000",
		"JOBTOOL_ROLE_FAMILIES":           `[{"slug":"backend","keywords":["go","golang"]}]`,
		"JOBTOOL_EMBEDDING_PROVIDER":      "local",
		"JOBTOOL_EMBEDDING_MODEL":         "local-hash-v1",
		"JOBTOOL_EMBEDDING_DIM":           "768",
		"JOBTOOL_LLM_PROVIDER":            "none",
		"JOBTOOL_LLM_TIMEOUT_S":           "30",
		"JOBTOOL_DIGEST_DEFAULT_FREQUENCY": "weekly",
		"JOBTOOL_DIGEST_SEND_HOUR":        "8",
		"JOBTOOL_SOURCES_TIMEOUT_S":       "30",
	}
}

func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadValidConfig(t *testing.T) {
	cfg, err := Load(envFunc(fullEnv()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Embedding.Dim != 768 {
		t.Fatalf("dim = %d", cfg.Embedding.Dim)
	}
	if !cfg.RoleFamilies.Valid("backend") {
		t.Fatal("backend family should be valid")
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{name: "missing database url", mutate: func(m map[string]string) { delete(m, "JOBTOOL_DATABASE_URL") }},
		{name: "bad role", mutate: func(m map[string]string) { m["JOBTOOL_ROLE"] = "cron" }},
		{name: "bad dim", mutate: func(m map[string]string) { m["JOBTOOL_EMBEDDING_DIM"] = "zero" }},
		{name: "llm without model", mutate: func(m map[string]string) { m["JOBTOOL_LLM_PROVIDER"] = "ollama"; m["JOBTOOL_LLM_BASE_URL"] = "http://ollama:11434" }},
		{name: "llm without base url", mutate: func(m map[string]string) { m["JOBTOOL_LLM_PROVIDER"] = "openai_compatible"; m["JOBTOOL_LLM_MODEL"] = "m" }},
		{name: "bad json families", mutate: func(m map[string]string) { m["JOBTOOL_ROLE_FAMILIES"] = "backend" }},
		{name: "send hour out of range", mutate: func(m map[string]string) { m["JOBTOOL_DIGEST_SEND_HOUR"] = "24" }},
		{name: "bad frequency", mutate: func(m map[string]string) { m["JOBTOOL_DIGEST_DEFAULT_FREQUENCY"] = "hourly" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := fullEnv()
			tt.mutate(env)
			if _, err := Load(envFunc(env)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestHealthEndpoint(t *testing.T) {
	log := NewLoggerTo(os.NewFile(0, os.DevNull), "error")
	if log == nil {
		log = slog.New(slog.NewJSONHandler(nil, nil))
	}

	tests := []struct {
		name       string
		check      HealthChecker
		wantCode   int
		wantStatus string
	}{
		{
			name:       "healthy",
			check:      func(context.Context) error { return nil },
			wantCode:   http.StatusOK,
			wantStatus: `"ok"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHTTPHandler(log, "*", map[string]HealthChecker{"postgres": tt.check})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d want %d", rec.Code, tt.wantCode)
			}
			if !strings.Contains(rec.Body.String(), tt.wantStatus) {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}
}

func TestHealthReportsDegraded(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.NewFile(0, os.DevNull), nil))
	h := NewHTTPHandler(log, "*", map[string]HealthChecker{
		"postgres": func(context.Context) error { return context.DeadlineExceeded },
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "degraded") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
