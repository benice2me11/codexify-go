package execsession

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	defaultYield = 10 * time.Second
	maxYield     = 30 * time.Second
	maxOutput    = 1 << 20
)

type StartInput struct {
	Command string
	Shell   string
	Workdir string
	Yield   time.Duration
}

type Result struct {
	Output    string
	SessionID string
	Running   bool
	ExitCode  *int
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*session
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*session)}
}

func (m *Manager) Start(in StartInput) (Result, error) {
	if strings.TrimSpace(in.Command) == "" {
		return Result{}, errors.New("command is required")
	}
	exe, args, err := shellCommand(in.Shell, in.Command)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = in.Workdir
	cmd.Env = os.Environ()
	configureProcess(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, err
	}
	s := &session{
		id:    newID(),
		cmd:   cmd,
		stdin: stdin,
		done:  make(chan error, 1),
	}
	cmd.Stdout = s
	cmd.Stderr = s

	if err := cmd.Start(); err != nil {
		return Result{}, err
	}
	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		s.done <- err
		close(s.done)
	}()

	return m.await(s, normalizeYield(in.Yield))
}

func (m *Manager) Write(id, chars string, yield time.Duration) (Result, error) {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil {
		return Result{}, fmt.Errorf("unknown session %q", id)
	}
	if chars == "\x03" {
		if err := stopProcessTree(s.cmd.Process.Pid); err != nil {
			select {
			case <-s.done:
			default:
				return Result{}, err
			}
		}
		return m.await(s, normalizeYield(yield))
	}
	if chars != "" {
		if _, err := io.WriteString(s.stdin, chars); err != nil {
			select {
			case <-s.done:
			default:
				return Result{}, err
			}
		}
	}
	return m.await(s, normalizeYield(yield))
}

func (m *Manager) Close() {
	m.mu.Lock()
	sessions := make([]*session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	for _, s := range sessions {
		_ = stopProcessTree(s.cmd.Process.Pid)
	}
}

func (m *Manager) await(s *session, yield time.Duration) (Result, error) {
	timer := time.NewTimer(yield)
	defer timer.Stop()
	select {
	case err := <-s.done:
		code := exitCode(err)
		m.mu.Lock()
		delete(m.sessions, s.id)
		m.mu.Unlock()
		_ = s.stdin.Close()
		return Result{
			Output:   s.drain(),
			Running:  false,
			ExitCode: &code,
		}, nil
	case <-timer.C:
		return Result{
			Output:    s.drain(),
			SessionID: s.id,
			Running:   true,
		}, nil
	}
}

type session struct {
	id    string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	done  chan error

	mu        sync.Mutex
	output    strings.Builder
	truncated bool
}

func (s *session) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := maxOutput - s.output.Len()
	if remaining <= 0 {
		s.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = s.output.Write(p[:remaining])
		s.truncated = true
		return len(p), nil
	}
	_, _ = s.output.Write(p)
	return len(p), nil
}

func (s *session) drain() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.output.String()
	s.output.Reset()
	if s.truncated {
		out += "\n[output truncated]\n"
		s.truncated = false
	}
	return out
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func normalizeYield(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultYield
	}
	if d > maxYield {
		return maxYield
	}
	return d
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func shellCommand(shell, command string) (string, []string, error) {
	shell = strings.ToLower(strings.TrimSpace(shell))
	if runtime.GOOS == "windows" {
		switch shell {
		case "", "powershell":
			return "powershell.exe", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}, nil
		case "pwsh":
			return "pwsh.exe", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}, nil
		case "cmd":
			return "cmd.exe", []string{"/d", "/s", "/c", command}, nil
		default:
			return "", nil, fmt.Errorf("unsupported shell %q", shell)
		}
	}
	switch shell {
	case "", "sh":
		return "/bin/sh", []string{"-lc", command}, nil
	case "bash":
		return "bash", []string{"-lc", command}, nil
	case "zsh":
		return "zsh", []string{"-lc", command}, nil
	default:
		return "", nil, fmt.Errorf("unsupported shell %q", shell)
	}
}

func (m *Manager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil {
		return fmt.Errorf("unknown session %q", id)
	}
	done := make(chan error, 1)
	go func() { done <- stopProcessTree(s.cmd.Process.Pid) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
