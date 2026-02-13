package logger

import (
	"log/slog"
	"os"
)

var defaultLogger *slog.Logger

func init() {
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// Get returns the default logger instance
func Get() *slog.Logger {
	return defaultLogger
}

// SetLevel reconfigures the logger with the given level
func SetLevel(level slog.Level) {
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

// SetJSON switches to JSON output format (useful for production/log aggregation)
func SetJSON(level slog.Level) {
	defaultLogger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}
