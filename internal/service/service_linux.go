//go:build linux

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/benice2me11/codexify-go/internal/app"
	"github.com/benice2me11/codexify-go/internal/config"
)

const (
	stateStopped = 1
	stateRunning = 2

	linuxServiceWaitTimeout  = 15 * time.Second
	linuxServicePollInterval = 200 * time.Millisecond
)

type Status struct {
	Installed bool
	State     int
	PID       int
}

func IsRunning(status Status) bool {
	return status.Installed && status.State == stateRunning
}

func StateString(state int) string {
	switch state {
	case stateStopped:
		return "stopped"
	case stateRunning:
		return "running"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

type linuxBackend struct {
	unitDir   func() (string, error)
	systemctl func(...string) (string, error)
}

var defaultLinuxBackend = linuxBackend{
	unitDir:   systemdUserUnitDir,
	systemctl: runSystemctl,
}

func Install(exePath, configPath string, cfg config.Config) error {
	return defaultLinuxBackend.install(exePath, configPath, cfg)
}

func Start(name string) error {
	return defaultLinuxBackend.start(name)
}

func Stop(name string) error {
	return defaultLinuxBackend.stop(name)
}

func Restart(name string) error {
	return defaultLinuxBackend.restart(name)
}

func Remove(name string) error {
	return defaultLinuxBackend.remove(name)
}

func Query(name string) (Status, error) {
	return defaultLinuxBackend.query(name)
}

func Run(_ string, cfg config.Config, configPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return app.Run(ctx, cfg, configPath, false)
}

func ExecutablePath() (string, error) {
	return os.Executable()
}

func (b linuxBackend) install(exePath, configPath string, cfg config.Config) error {
	exePath, err := filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	dir, err := b.unitDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create systemd user unit directory: %w", err)
	}

	unitName := systemdUnitName(cfg.Service.Name)
	unitPath, err := linuxUnitPath(dir, cfg.Service.Name)
	if err != nil {
		return err
	}
	unit, err := renderSystemdUnit(exePath, configPath, cfg)
	if err != nil {
		return err
	}

	previous, previousMode, existed, err := readManagedUnit(unitPath, unitName)
	if err != nil {
		return err
	}
	if err := writeManagedUnit(unitPath, unitName, []byte(unit)); err != nil {
		return err
	}

	rollback := func() {
		if existed {
			_ = atomicWriteFile(unitPath, previous, previousMode)
		} else {
			_ = os.Remove(unitPath)
		}
		_, _ = b.systemctl("daemon-reload")
	}

	out, err := b.systemctl("daemon-reload")
	if err != nil {
		rollback()
		return systemctlFailure("daemon-reload", out, err)
	}
	out, err = b.systemctl("enable", unitName)
	if err != nil {
		if !existed {
			_, _ = b.systemctl("disable", unitName)
		}
		rollback()
		return systemctlFailure("enable "+unitName, out, err)
	}
	return nil
}

func (b linuxBackend) start(name string) error {
	status, err := b.query(name)
	if err != nil {
		return err
	}
	if !status.Installed {
		return fmt.Errorf("service %q is not installed", name)
	}
	if IsRunning(status) {
		return nil
	}

	unitName := systemdUnitName(name)
	out, err := b.systemctl("start", unitName)
	if err != nil {
		return systemctlFailure("start "+unitName, out, err)
	}
	return b.waitRunning(name, true, linuxServiceWaitTimeout)
}

func (b linuxBackend) stop(name string) error {
	status, err := b.query(name)
	if err != nil {
		return err
	}
	if !status.Installed {
		return nil
	}

	unitName := systemdUnitName(name)
	out, err := b.systemctl("stop", unitName)
	if err != nil {
		return systemctlFailure("stop "+unitName, out, err)
	}
	return b.waitRunning(name, false, linuxServiceWaitTimeout)
}

func (b linuxBackend) restart(name string) error {
	status, err := b.query(name)
	if err != nil {
		return err
	}
	if !status.Installed {
		return fmt.Errorf("service %q is not installed", name)
	}

	unitName := systemdUnitName(name)
	out, err := b.systemctl("restart", unitName)
	if err != nil {
		return systemctlFailure("restart "+unitName, out, err)
	}
	return b.waitRunning(name, true, linuxServiceWaitTimeout)
}

func (b linuxBackend) remove(name string) error {
	dir, err := b.unitDir()
	if err != nil {
		return err
	}
	unitName := systemdUnitName(name)
	unitPath, err := linuxUnitPath(dir, name)
	if err != nil {
		return err
	}

	if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat systemd user unit: %w", err)
	}
	if _, _, _, err := readManagedUnit(unitPath, unitName); err != nil {
		return err
	}

	if err := b.stop(name); err != nil {
		return fmt.Errorf("stop service before removal: %w", err)
	}
	out, err := b.systemctl("disable", unitName)
	if err != nil {
		return systemctlFailure("disable "+unitName, out, err)
	}
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove systemd user unit: %w", err)
	}
	out, err = b.systemctl("daemon-reload")
	if err != nil {
		return systemctlFailure("daemon-reload after removal", out, err)
	}
	return nil
}

func (b linuxBackend) query(name string) (Status, error) {
	dir, err := b.unitDir()
	if err != nil {
		return Status{}, err
	}
	unitPath, err := linuxUnitPath(dir, name)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
		return Status{Installed: false, State: stateStopped}, nil
	} else if err != nil {
		return Status{}, fmt.Errorf("stat systemd user unit: %w", err)
	}

	unitName := systemdUnitName(name)
	out, err := b.systemctl(
		"show",
		unitName,
		"--property=LoadState",
		"--property=ActiveState",
		"--property=SubState",
		"--property=MainPID",
		"--property=Result",
		"--no-pager",
	)
	if err != nil {
		return Status{}, systemctlFailure("show "+unitName, out, err)
	}
	state, err := parseSystemctlShow(out)
	if err != nil {
		return Status{}, fmt.Errorf("parse systemctl state for %s: %w", unitName, err)
	}

	runtimeState := stateStopped
	if state.ActiveState == "active" && state.MainPID > 0 {
		runtimeState = stateRunning
	}
	return Status{
		Installed: true,
		State:     runtimeState,
		PID:       state.MainPID,
	}, nil
}

func (b linuxBackend) waitRunning(name string, want bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		status, err := b.query(name)
		if err != nil {
			return err
		}
		if IsRunning(status) == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service %q did not reach running=%t within %s", name, want, timeout)
		}
		time.Sleep(linuxServicePollInterval)
	}
}

func systemdUserUnitDir() (string, error) {
	if os.Geteuid() == 0 {
		return "", errors.New("Codexify Go Linux service must be managed as the logged-in user, not root")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	if !filepath.IsAbs(configDir) {
		return "", fmt.Errorf("user config directory must be absolute: %q", configDir)
	}
	return filepath.Join(configDir, "systemd", "user"), nil
}

func linuxUnitPath(unitDir, serviceName string) (string, error) {
	if !filepath.IsAbs(unitDir) {
		return "", fmt.Errorf("systemd user unit directory must be absolute: %q", unitDir)
	}
	base := filepath.Clean(unitDir)
	path := filepath.Join(base, systemdUnitName(serviceName))
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", fmt.Errorf("resolve systemd unit path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("systemd unit path escapes user unit directory: %q", path)
	}
	return path, nil
}

func systemdUnitName(name string) string {
	canonical := strings.TrimSpace(name)
	if canonical == "" {
		canonical = "CodexifyGo"
	}
	sum := sha256.Sum256([]byte(canonical))

	var slug strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(canonical) {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			if slug.Len() >= 48 {
				break
			}
			slug.WriteRune(r)
			lastDash = false
			continue
		}
		if slug.Len() > 0 && !lastDash && slug.Len() < 48 {
			slug.WriteByte('-')
			lastDash = true
		}
	}
	slugText := strings.Trim(slug.String(), "-")
	if slugText == "" {
		slugText = "service"
	}
	return fmt.Sprintf("codexify-go-%s-%x.service", slugText, sum[:6])
}

func renderSystemdUnit(exePath, configPath string, cfg config.Config) (string, error) {
	if !filepath.IsAbs(exePath) {
		return "", fmt.Errorf("service executable path must be absolute: %q", exePath)
	}
	if !filepath.IsAbs(configPath) {
		return "", fmt.Errorf("service config path must be absolute: %q", configPath)
	}

	args := []string{exePath, "service", "run", "--config", configPath}
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		value, err := systemdQuoteArg(arg)
		if err != nil {
			return "", fmt.Errorf("quote systemd ExecStart argument: %w", err)
		}
		quoted = append(quoted, value)
	}

	timeout := cfg.Supervisor.ShutdownTimeout.Duration() + 2*time.Second
	if timeout <= 0 {
		return "", errors.New("supervisor.shutdownTimeout must produce a positive systemd stop timeout")
	}

	unitName := systemdUnitName(cfg.Service.Name)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", managedUnitHeader(unitName))
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Codexify Go\n")
	b.WriteString("StartLimitIntervalSec=60s\n")
	b.WriteString("StartLimitBurst=5\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=exec\n")
	fmt.Fprintf(&b, "ExecStart=:%s\n", strings.Join(quoted, " "))
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=5s\n")
	b.WriteString("KillMode=control-group\n")
	fmt.Fprintf(&b, "TimeoutStopSec=%dms\n", timeout.Milliseconds())
	b.WriteString("StandardOutput=journal\n")
	b.WriteString("StandardError=journal\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=default.target\n")
	return b.String(), nil
}

func systemdQuoteArg(value string) (string, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return "", errors.New("systemd arguments cannot contain NUL")
	}

	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch c {
		case '%':
			b.WriteString("%%")
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		default:
			if c < 0x20 || c == 0x7f {
				fmt.Fprintf(&b, "\\x%02x", c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

type systemctlShowState struct {
	LoadState   string
	ActiveState string
	SubState    string
	MainPID     int
	Result      string
}

func parseSystemctlShow(out string) (systemctlShowState, error) {
	var state systemctlShowState
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return systemctlShowState{}, fmt.Errorf("invalid systemctl property line %q", line)
		}
		switch key {
		case "LoadState":
			state.LoadState = value
		case "ActiveState":
			state.ActiveState = value
		case "SubState":
			state.SubState = value
		case "MainPID":
			if value == "" {
				continue
			}
			pid, err := strconv.Atoi(value)
			if err != nil || pid < 0 {
				return systemctlShowState{}, fmt.Errorf("invalid MainPID %q", value)
			}
			state.MainPID = pid
		case "Result":
			state.Result = value
		}
	}
	return state, nil
}

func managedUnitHeader(unitName string) string {
	return "# Managed by Codexify Go: " + unitName
}

func readManagedUnit(path, unitName string) (data []byte, mode os.FileMode, exists bool, err error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("stat existing systemd user unit: %w", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, 0, false, fmt.Errorf("read existing systemd user unit: %w", err)
	}
	if !bytes.HasPrefix(data, []byte(managedUnitHeader(unitName)+"\n")) {
		return nil, 0, false, fmt.Errorf("refusing to overwrite or remove unrelated systemd user unit %q", path)
	}
	return data, info.Mode().Perm(), true, nil
}

func writeManagedUnit(path, unitName string, data []byte) error {
	if !bytes.HasPrefix(data, []byte(managedUnitHeader(unitName)+"\n")) {
		return errors.New("managed systemd unit is missing ownership marker")
	}
	if _, _, _, err := readManagedUnit(path, unitName); err != nil {
		return err
	}
	if err := atomicWriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write systemd user unit: %w", err)
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".codexify-go-unit-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func runSystemctl(args ...string) (string, error) {
	cmdArgs := append([]string{"--user"}, args...)
	cmd := exec.Command("systemctl", cmdArgs...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func systemctlFailure(action, out string, err error) error {
	message := strings.TrimSpace(out)
	if message == "" {
		return fmt.Errorf("systemctl --user %s: %w", action, err)
	}
	return fmt.Errorf("systemctl --user %s: %w: %s", action, err, message)
}
