package platform

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewLogger returns a JSON slog logger at the configured level.
func NewLogger(level string) *slog.Logger { return NewLoggerTo(os.Stdout, level) }

// NewLoggerTo is NewLogger with an explicit destination, for tests.
func NewLoggerTo(w io.Writer, level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl}))
}
