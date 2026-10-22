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

// Route is one method+pattern registration for the Go 1.22 pattern mux.
type Route struct {
	Pattern string
	Handler http.HandlerFunc
}

// NewHTTPHandler builds the API mux from routes.
func NewHTTPHandler(log *slog.Logger, webOrigin string, routes []Route) http.Handler {
	mux := http.NewServeMux()
	for _, r := range routes {
		mux.HandleFunc(r.Pattern, r.Handler)
	}
	return withCORS(webOrigin, withRequestLog(log, mux))
}

// HealthHandler reports overall health from the named checks.
func HealthHandler(log *slog.Logger, checks map[string]HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := struct {
			Status string            `json:"status"`
			Checks map[string]string `json:"checks,omitempty"`
		}{Status: "ok", Checks: map[string]string{}}
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
		WriteJSON(w, code, status)
	}
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
