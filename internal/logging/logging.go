// Package logging keeps the supervisor and user worker on the same log policy.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

func NewJSON(out io.Writer, configuredLevel string) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(configuredLevel)) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level}))
}
