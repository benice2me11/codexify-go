//go:build windows

package supervisor

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

func TestAttachProcessJobKillsProcessWhenReleased(t *testing.T) {
	cmd := exec.Command(
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		"Start-Sleep -Seconds 30",
	)
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	release, err := attachProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	release()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process survived closing kill-on-close job")
	}
}

func TestCommandProcessStopsHiddenChild(t *testing.T) {
	factory := &CommandFactory{
		Spec: CommandSpec{
			Path: "cmd.exe",
			Args: []string{"/d", "/s", "/c", "ping -t 127.0.0.1 >nul"},
		},
		Output: io.Discard,
	}
	proc, err := factory.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if proc.PID() <= 0 {
		t.Fatalf("invalid pid %d", proc.PID())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := proc.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.Done():
	case <-time.After(time.Second):
		t.Fatal("child did not exit")
	}
}

type countingFactory struct {
	inner  Factory
	starts atomic.Int32
}

func (f *countingFactory) Start(ctx context.Context) (Process, error) {
	proc, err := f.inner.Start(ctx)
	if err == nil {
		f.starts.Add(1)
	}
	return proc, err
}

func TestSupervisorRestartsRealExitedProcess(t *testing.T) {
	inner := &CommandFactory{
		Spec: CommandSpec{
			Path: "cmd.exe",
			Args: []string{"/d", "/s", "/c", "exit 17"},
		},
		Output: io.Discard,
	}
	factory := &countingFactory{inner: inner}
	// Windows GitHub runners can need substantially more than 150 ms for
	// three separate cmd.exe process launches. Await the *event* we are
	// testing instead of assuming a fixed wall-clock number of launches.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s := &Supervisor{
		Factory: factory,
		Policy: Policy{
			MinBackoff:             5 * time.Millisecond,
			MaxBackoff:             20 * time.Millisecond,
			StableWindow:           time.Second,
			HealthInterval:         time.Second,
			HealthFailureThreshold: 3,
			ShutdownTimeout:        100 * time.Millisecond,
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	finished := make(chan error, 1)
	go func() { finished <- s.Run(ctx) }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for factory.starts.Load() < 3 {
		select {
		case err := <-finished:
			t.Fatalf("supervisor exited before the third successful launch: %v; starts=%d", err, factory.starts.Load())
		case <-ctx.Done():
			t.Fatalf("timed out waiting for three successful real process launches: starts=%d", factory.starts.Load())
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not stop after cancellation")
	}
}
