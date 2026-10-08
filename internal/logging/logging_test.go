package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestConfiguredLevels(t *testing.T) {
	for _, tc := range []struct {
		name    string
		minimum slog.Level
	}{
		{"debug", slog.LevelDebug}, {" DEBUG ", slog.LevelDebug},
		{"info", slog.LevelInfo}, {"", slog.LevelInfo}, {"unrecognized", slog.LevelInfo},
		{"warn", slog.LevelWarn}, {"WARNING", slog.LevelWarn}, {"error", slog.LevelError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewJSON(&buf, tc.name)
			want := 0
			for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
				if got := logger.Enabled(context.Background(), level); got != (level >= tc.minimum) {
					t.Fatalf("Enabled(%v)=%v", level, got)
				}
				logger.Log(context.Background(), level, "fixture")
				if level >= tc.minimum {
					want++
				}
			}
			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			if len(lines) != want {
				t.Fatalf("got %d records, want %d", len(lines), want)
			}
			for _, line := range lines {
				if !json.Valid([]byte(line)) {
					t.Fatal("not JSONL")
				}
			}
		})
	}
}
