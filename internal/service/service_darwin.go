//go:build darwin

package service

import (
	"bytes"
	"context"
	"encoding/xml"
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

func Install(exePath, configPath string, cfg config.Config) error {
	exePath, err := filepath.Abs(exePath)
	if err != nil {
		return err
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return err
	}

	label := launchdLabel(cfg.Service.Name)
	plistPath, err := plistPath(label)
	if err != nil {
		return err
	}
	if _, err := os.Stat(plistPath); err == nil {
		return fmt.Errorf("service %q already exists", cfg.Service.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Log.File), 0o755); err != nil {
		return err
	}

	plist := launchdPlist{
		Label: label,
		ProgramArguments: []string{
			exePath,
			"service",
			"run",
			"--config",
			configPath,
		},
		RunAtLoad:         true,
		KeepAlive:         launchdKeepAlive{SuccessfulExit: boolPtr(false)},
		ProcessType:       "Background",
		ThrottleInterval:  5,
		StandardOutPath:   cfg.Log.File + ".launchd.out",
		StandardErrorPath: cfg.Log.File + ".launchd.err",
	}
	data := []byte(plistXML(plist))
	return os.WriteFile(plistPath, data, 0o644)
}

func Start(name string) error {
	label := launchdLabel(name)
	plistPath, err := plistPath(label)
	if err != nil {
		return err
	}
	if _, err := os.Stat(plistPath); err != nil {
		return fmt.Errorf("service %q is not installed", name)
	}
	domain, err := userDomain()
	if err != nil {
		return err
	}
	loaded := jobExists(domain, label)
	if !loaded {
		if out, err := launchctl("bootstrap", domain, plistPath); err != nil {
			return fmt.Errorf("bootstrap launch agent: %w: %s", err, strings.TrimSpace(out))
		}
	} else {
		if out, err := launchctl("kickstart", "-k", domain+"/"+label); err != nil {
			return fmt.Errorf("kickstart launch agent: %w: %s", err, strings.TrimSpace(out))
		}
	}
	return waitRunning(name, true, 15*time.Second)
}

func Stop(name string) error {
	label := launchdLabel(name)
	domain, err := userDomain()
	if err != nil {
		return err
	}
	if !jobExists(domain, label) {
		return nil
	}
	if out, err := launchctl("bootout", domain+"/"+label); err != nil {
		return fmt.Errorf("bootout launch agent: %w: %s", err, strings.TrimSpace(out))
	}
	return waitRunning(name, false, 15*time.Second)
}

func Restart(name string) error {
	if err := Stop(name); err != nil {
		return err
	}
	return Start(name)
}

func Remove(name string) error {
	if err := Stop(name); err != nil {
		return fmt.Errorf("stop service before removal: %w", err)
	}
	label := launchdLabel(name)
	plistPath, err := plistPath(label)
	if err != nil {
		return err
	}
	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func Query(name string) (Status, error) {
	label := launchdLabel(name)
	plistPath, err := plistPath(label)
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(plistPath); errors.Is(err, os.ErrNotExist) {
		return Status{Installed: false, State: stateStopped}, nil
	} else if err != nil {
		return Status{}, err
	}
	domain, err := userDomain()
	if err != nil {
		return Status{}, err
	}
	out, err := launchctl("print", domain+"/"+label)
	if err != nil {
		return Status{Installed: true, State: stateStopped}, nil
	}
	pid := parseLaunchctlPID(out)
	state := stateStopped
	if strings.Contains(out, "state = running") || pid > 0 {
		state = stateRunning
	}
	return Status{Installed: true, State: state, PID: pid}, nil
}

func Run(_ string, cfg config.Config, configPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return app.Run(ctx, cfg, configPath, false)
}

func ExecutablePath() (string, error) {
	return os.Executable()
}

type launchdPlist struct {
	Label             string
	ProgramArguments  []string
	RunAtLoad         bool
	KeepAlive         launchdKeepAlive
	ProcessType       string
	ThrottleInterval  int
	StandardOutPath   string
	StandardErrorPath string
}

type launchdKeepAlive struct {
	SuccessfulExit *bool
}

func plistXML(p launchdPlist) string {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	writeKV := func(key, value string) {
		fmt.Fprintf(&b, "\t<key>%s</key>\n\t<string>%s</string>\n", xmlEscape(key), xmlEscape(value))
	}
	writeKV("Label", p.Label)
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, arg := range p.ProgramArguments {
		fmt.Fprintf(&b, "\t\t<string>%s</string>\n", xmlEscape(arg))
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n")
	if p.RunAtLoad {
		b.WriteString("\t<true/>\n")
	} else {
		b.WriteString("\t<false/>\n")
	}
	b.WriteString("\t<key>KeepAlive</key>\n\t<dict>\n")
	if p.KeepAlive.SuccessfulExit != nil {
		b.WriteString("\t\t<key>SuccessfulExit</key>\n")
		if *p.KeepAlive.SuccessfulExit {
			b.WriteString("\t\t<true/>\n")
		} else {
			b.WriteString("\t\t<false/>\n")
		}
	}
	b.WriteString("\t</dict>\n")
	writeKV("ProcessType", p.ProcessType)
	fmt.Fprintf(&b, "\t<key>ThrottleInterval</key>\n\t<integer>%d</integer>\n", p.ThrottleInterval)
	writeKV("StandardOutPath", p.StandardOutPath)
	writeKV("StandardErrorPath", p.StandardErrorPath)
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func xmlEscape(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func launchdLabel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "CodexifyGo"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	suffix := strings.ToLower(strings.Trim(b.String(), ".-_"))
	if suffix == "" {
		suffix = "service"
	}
	return "io.github.benice2me11.codexify-go." + suffix
}

func plistPath(label string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("cannot resolve user home for LaunchAgent")
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func userDomain() (string, error) {
	uid := os.Getuid()
	if uid <= 0 {
		return "", errors.New("Codexify Go macOS service must be installed as the logged-in user, not root")
	}
	return "gui/" + strconv.Itoa(uid), nil
}

func launchctl(args ...string) (string, error) {
	cmd := exec.Command("/bin/launchctl", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func jobExists(domain, label string) bool {
	_, err := launchctl("print", domain+"/"+label)
	return err == nil
}

func parseLaunchctlPID(out string) int {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "pid = ") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "pid = "))
		pid, _ := strconv.Atoi(value)
		return pid
	}
	return 0
}

func waitRunning(name string, want bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := Query(name)
		if err != nil {
			return err
		}
		if IsRunning(status) == want {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("service %q did not reach running=%t within %s", name, want, timeout)
}

func boolPtr(value bool) *bool { return &value }
