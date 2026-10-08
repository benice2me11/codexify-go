package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/health"
	"github.com/benice2me11/codexify-go/internal/logging"
	"github.com/benice2me11/codexify-go/internal/mcpserver"
	"github.com/benice2me11/codexify-go/internal/supervisor"
	"github.com/benice2me11/codexify-go/internal/tunnel"
	"github.com/benice2me11/codexify-go/internal/userworker"
)

func Run(ctx context.Context, cfg config.Config, configPath string, console bool) error {
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

	logger.Info("codexify-go starting",
		"service", cfg.Service.Name,
		"tunnel_id", cfg.Tunnel.TunnelID,
		"tunnel_executable", cfg.Tunnel.Executable,
	)

	if runtime.GOOS == "windows" && !console {
		return runWithUserWorker(ctx, cfg, configPath, logger, logFile)
	}
	return runInProcess(ctx, cfg, logger, logFile)
}

func runInProcess(ctx context.Context, cfg config.Config, logger *slog.Logger, logFile io.Writer) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	mcpRuntime, err := mcpserver.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("start MCP runtime: %w", err)
	}
	authRef, authEnv := mcpRuntime.TunnelEnvironment()
	mcpserver.MergeTunnelEnvironment(&cfg, authRef, authEnv)

	factory := &tunnel.Factory{Config: cfg, Output: logFile}
	checker := health.NewURLFileChecker(cfg.Tunnel.HealthURLFile)
	s := &supervisor.Supervisor{
		Factory: factory,
		Health:  checker,
		Policy: supervisor.Policy{
			MinBackoff:             cfg.Supervisor.MinBackoff.Duration(),
			MaxBackoff:             cfg.Supervisor.MaxBackoff.Duration(),
			StableWindow:           cfg.Supervisor.StableWindow.Duration(),
			StartupGrace:           cfg.Tunnel.StartupWaitTimeout.Duration() + cfg.Supervisor.HealthInterval.Duration(),
			HealthInterval:         cfg.Supervisor.HealthInterval.Duration(),
			HealthFailureThreshold: cfg.Supervisor.HealthFailureThreshold,
			ShutdownTimeout:        cfg.Supervisor.ShutdownTimeout.Duration(),
		},
		Log: logger,
	}

	mcpErr := make(chan error, 1)
	go func() { mcpErr <- mcpRuntime.Serve() }()
	supervisorErr := make(chan error, 1)
	go func() { supervisorErr <- s.Run(runCtx) }()

	var runErr error
	supervisorFinished := false
	mcpFinished := false
	select {
	case <-ctx.Done():
	case err := <-mcpErr:
		mcpFinished = true
		if err != nil {
			runErr = fmt.Errorf("MCP server stopped: %w", err)
		}
	case err := <-supervisorErr:
		supervisorFinished = true
		if err != nil {
			runErr = fmt.Errorf("tunnel supervisor stopped: %w", err)
		}
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Supervisor.ShutdownTimeout.Duration())
	defer shutdownCancel()
	if err := mcpRuntime.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = fmt.Errorf("shutdown MCP server: %w", err)
	}
	if !supervisorFinished {
		select {
		case err := <-supervisorErr:
			if err != nil && runErr == nil {
				runErr = fmt.Errorf("shutdown tunnel supervisor: %w", err)
			}
		case <-shutdownCtx.Done():
			if runErr == nil {
				runErr = errors.New("timed out waiting for tunnel supervisor shutdown")
			}
		}
	}
	if !mcpFinished {
		select {
		case err := <-mcpErr:
			if err != nil && runErr == nil {
				runErr = fmt.Errorf("MCP server shutdown: %w", err)
			}
		default:
		}
	}
	if runErr != nil {
		logger.Error("codexify-go stopped with error", "error", runErr)
		return runErr
	}
	logger.Info("codexify-go stopped")
	return nil
}

func runWithUserWorker(ctx context.Context, cfg config.Config, configPath string, logger *slog.Logger, logFile io.Writer) error {
	token := ""
	if cfg.MCP.AuthEnabled {
		var err error
		token, err = mcpserver.GenerateToken()
		if err != nil {
			return err
		}
	}
	authRef, authEnv := mcpserver.AuthEnvironment(token)
	mcpserver.MergeTunnelEnvironment(&cfg, authRef, authEnv)

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	workerHealthURL, err := mcpserver.HealthURL(cfg.Tunnel.MCPServerURL)
	if err != nil {
		return err
	}

	policy := supervisor.Policy{
		MinBackoff:             cfg.Supervisor.MinBackoff.Duration(),
		MaxBackoff:             cfg.Supervisor.MaxBackoff.Duration(),
		StableWindow:           cfg.Supervisor.StableWindow.Duration(),
		StartupGrace:           2 * cfg.Supervisor.HealthInterval.Duration(),
		HealthInterval:         cfg.Supervisor.HealthInterval.Duration(),
		HealthFailureThreshold: cfg.Supervisor.HealthFailureThreshold,
		ShutdownTimeout:        cfg.Supervisor.ShutdownTimeout.Duration(),
	}
	workerSupervisor := &supervisor.Supervisor{
		Factory: &userworker.Factory{
			Executable: executable,
			ConfigPath: configPath,
			Env:        authEnv,
			Log:        logger,
		},
		Health: health.NewHTTPChecker(workerHealthURL),
		Policy: policy,
		Log:    logger.With("component", "user_worker"),
	}
	tunnelSupervisor := &supervisor.Supervisor{
		Factory: &tunnel.Factory{Config: cfg, Output: logFile},
		Health:  health.NewURLFileChecker(cfg.Tunnel.HealthURLFile),
		Policy: supervisor.Policy{
			MinBackoff:             cfg.Supervisor.MinBackoff.Duration(),
			MaxBackoff:             cfg.Supervisor.MaxBackoff.Duration(),
			StableWindow:           cfg.Supervisor.StableWindow.Duration(),
			StartupGrace:           cfg.Tunnel.StartupWaitTimeout.Duration() + cfg.Supervisor.HealthInterval.Duration(),
			HealthInterval:         cfg.Supervisor.HealthInterval.Duration(),
			HealthFailureThreshold: cfg.Supervisor.HealthFailureThreshold,
			ShutdownTimeout:        cfg.Supervisor.ShutdownTimeout.Duration(),
		},
		Log: logger.With("component", "tunnel"),
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workerErr := make(chan error, 1)
	tunnelErr := make(chan error, 1)
	go func() { workerErr <- workerSupervisor.Run(runCtx) }()
	go func() { tunnelErr <- tunnelSupervisor.Run(runCtx) }()

	var runErr error
	workerFinished := false
	tunnelFinished := false
	select {
	case <-ctx.Done():
	case err := <-workerErr:
		workerFinished = true
		if err != nil {
			runErr = fmt.Errorf("user worker supervisor stopped: %w", err)
		}
	case err := <-tunnelErr:
		tunnelFinished = true
		if err != nil {
			runErr = fmt.Errorf("tunnel supervisor stopped: %w", err)
		}
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Supervisor.ShutdownTimeout.Duration()+2*cfg.Supervisor.HealthInterval.Duration())
	defer shutdownCancel()
	if !workerFinished {
		select {
		case err := <-workerErr:
			if err != nil && runErr == nil {
				runErr = fmt.Errorf("shutdown user worker supervisor: %w", err)
			}
		case <-shutdownCtx.Done():
			if runErr == nil {
				runErr = errors.New("timed out waiting for user worker supervisor shutdown")
			}
		}
	}
	if !tunnelFinished {
		select {
		case err := <-tunnelErr:
			if err != nil && runErr == nil {
				runErr = fmt.Errorf("shutdown tunnel supervisor: %w", err)
			}
		case <-shutdownCtx.Done():
			if runErr == nil {
				runErr = errors.New("timed out waiting for tunnel supervisor shutdown")
			}
		}
	}
	if runErr != nil {
		logger.Error("codexify-go stopped with error", "error", runErr)
		return runErr
	}
	logger.Info("codexify-go stopped")
	return nil
}
