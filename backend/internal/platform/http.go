package platform

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// HealthChecker reports whether a dependency is ready.
type HealthChecker func(ctx context.Context) error

// NewHTTPHandler builds the API mux. Routes use the Go 1.22 pattern mux.
func NewHTTPHandler(log *slog.Logger, webOrigin string, checks map[string]HealthChecker) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(log, checks))
	mux.HandleFunc("GET /api/health", healthHandler(log, checks))
	return withCORS(webOrigin, withRequestLog(log, mux))
}

type healthStatus struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func healthHandler(log *slog.Logger, checks map[string]HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := healthStatus{Status: "ok", Checks: map[string]string{}}
		code := http.StatusOK
		for name, check := range checks {
			if err := check(ctx); err != nil {
				status.Status = "degraded"
				status.Checks[name] = "error"
				code = http.StatusServiceUnavailable
				log.Warn("health check failed", "check", name, "error", err)
				continue
			}
			status.Checks[name] = "ok"
		}
		writeJSON(w, code, status)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
