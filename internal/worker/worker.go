package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/logging"
	"github.com/benice2me11/codexify-go/internal/mcpserver"
)

func Run(ctx context.Context, cfg config.Config, console bool) error {
	if err := os.MkdirAll(filepath.Dir(cfg.Log.File), 0o755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	var out io.Writer = logFile
	if console {
		out = io.MultiWriter(os.Stderr, logFile)
	}
	logger := logging.NewJSON(out, cfg.Log.Level)

	token := ""
	if cfg.MCP.AuthEnabled {
		header := os.Getenv(mcpserver.InternalAuthEnv)
		if !strings.HasPrefix(header, "Bearer ") || len(header) <= len("Bearer ") {
			return errors.New("worker requires internal MCP bearer environment")
		}
		token = strings.TrimPrefix(header, "Bearer ")
	}

	current := "unknown"
	if u, err := user.Current(); err == nil {
		current = u.Username
	}
	logger.Info("user worker starting", "user", current, "workspace", cfg.MCP.WorkspaceRoot)

	runtime, err := mcpserver.NewWithToken(cfg, logger, token)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- runtime.Serve() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Supervisor.ShutdownTimeout.Duration())
		defer cancel()
		return runtime.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
