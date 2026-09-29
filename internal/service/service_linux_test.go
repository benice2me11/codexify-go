//go:build linux

package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
)

func TestSystemdUnitNameIsDeterministicBoundedAndSafe(t *testing.T) {
	t.Parallel()

	name := "../../ Codexify Go / %prod 🔥"
	got := systemdUnitName(name)
	if got != systemdUnitName(name) {
		t.Fatalf("unit name is not deterministic: %q", got)
	}
	if !strings.HasPrefix(got, "codexify-go-") || !strings.HasSuffix(got, ".service") {
		t.Fatalf("unexpected unit name %q", got)
	}
	if strings.ContainsAny(got, "/\\ %") {
		t.Fatalf("unit name contains unsafe path/unit characters: %q", got)
	}
	if len(got) > 96 {
		t.Fatalf("unit name is not bounded: len=%d name=%q", len(got), got)
	}

	longName := strings.Repeat("Very Long Service Name ", 40)
	if got := systemdUnitName(longName); len(got) > 96 {
		t.Fatalf("long unit name is not bounded: len=%d name=%q", len(got), got)
	}

	// Similar slugs must not collapse to the same unit.
	if systemdUnitName("name with spaces") == systemdUnitName("name-with-spaces") {
		t.Fatal("distinct service names collided after normalization")
	}
}

func TestLinuxUnitPathCannotEscapeUnitDirectory(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	path, err := linuxUnitPath(base, "../../../../tmp/owned")
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		t.Fatal(err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("unit path escaped base directory: base=%q path=%q", base, path)
	}
	if filepath.Ext(path) != ".service" {
		t.Fatalf("expected service unit path, got %q", path)
	}
}

func TestRenderSystemdUnitUsesUserServiceContractAndEscapesPaths(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Supervisor.ShutdownTimeout = config.Duration(10 * time.Second)
	cfg.Tunnel.Environment = map[string]string{"SUPER_SECRET": "do-not-leak"}

	exe := `/opt/Codexify Go/bin/codexify%go$prod`
	configPath := `/home/test/Config Files/codexify%prod$1.json`
	unit, err := renderSystemdUnit(exe, configPath, cfg)
	if err != nil {
		t.Fatal(err)
	}

	required := []string{
		"StartLimitIntervalSec=60s",
		"StartLimitBurst=5",
		"[Service]",
		"Type=exec",
		"Restart=on-failure",
		"RestartSec=5s",
		"KillMode=control-group",
		"TimeoutStopSec=12000ms",
		"StandardOutput=journal",
		"StandardError=journal",
		"[Install]",
		"WantedBy=default.target",
		`ExecStart=:"/opt/Codexify Go/bin/codexify%%go$prod" "service" "run" "--config" "/home/test/Config Files/codexify%%prod$1.json"`,
	}
	for _, want := range required {
		if !strings.Contains(unit, want) {
			t.Errorf("rendered unit missing %q:\n%s", want, unit)
		}
	}

	for _, forbidden := range []string{"User=", "network-online.target", "/bin/sh", "do-not-leak", "SUPER_SECRET"} {
		if strings.Contains(unit, forbidden) {
			t.Errorf("rendered unit unexpectedly contains %q:\n%s", forbidden, unit)
		}
	}
}

func TestSystemdQuoteArgEscapesSystemdExpansionAndSyntax(t *testing.T) {
	t.Parallel()

	got, err := systemdQuoteArg("a b%u$HOME\\\"\n")
	if err != nil {
		t.Fatal(err)
	}
	want := `"a b%%u$HOME\\\"\x0a"`
	if got != want {
		t.Fatalf("quote mismatch:\n got: %s\nwant: %s", got, want)
	}

	if _, err := systemdQuoteArg("nul\x00byte"); err == nil {
		t.Fatal("expected NUL argument to be rejected")
	}
}

func TestParseSystemctlShow(t *testing.T) {
	t.Parallel()

	got, err := parseSystemctlShow("LoadState=loaded\nActiveState=active\nSubState=running\nMainPID=4242\nResult=success\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.LoadState != "loaded" || got.ActiveState != "active" || got.SubState != "running" || got.MainPID != 4242 || got.Result != "success" {
		t.Fatalf("unexpected parsed state: %#v", got)
	}

	if _, err := parseSystemctlShow("LoadState=loaded\nMainPID=not-a-number\n"); err == nil {
		t.Fatal("expected invalid MainPID to fail")
	}
}

func TestWriteManagedUnitIsAtomicAndRefusesUnrelatedOverwrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "codexify-go-test.service")
	unitName := filepath.Base(path)
	first := managedUnitHeader(unitName) + "\n[Service]\nType=exec\n"
	second := managedUnitHeader(unitName) + "\n[Service]\nType=exec\nRestart=on-failure\n"

	if err := writeManagedUnit(path, unitName, []byte(first)); err != nil {
		t.Fatal(err)
	}
	if err := writeManagedUnit(path, unitName, []byte(second)); err != nil {
		t.Fatalf("managed overwrite should be allowed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != second {
		t.Fatalf("unexpected managed unit contents:\n%s", data)
	}

	if err := os.WriteFile(path, []byte("[Unit]\nDescription=unrelated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeManagedUnit(path, unitName, []byte(first)); err == nil {
		t.Fatal("expected unrelated unit overwrite to be refused")
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), managedUnitHeader(unitName)) {
		t.Fatal("unrelated unit was overwritten")
	}
}

func TestLinuxBackendInstallWritesEnablesButDoesNotStart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var calls []string
	b := linuxBackend{
		unitDir: func() (string, error) { return dir, nil },
		systemctl: func(args ...string) (string, error) {
			calls = append(calls, strings.Join(args, " "))
			return "", nil
		},
	}
	cfg := config.Default()
	cfg.Service.Name = "Smoke Service"

	if err := b.install("/opt/codexify go", "/tmp/config file.json", cfg); err != nil {
		t.Fatal(err)
	}

	unitName := systemdUnitName(cfg.Service.Name)
	unitPath := filepath.Join(dir, unitName)
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), managedUnitHeader(unitName)+"\n") {
		t.Fatalf("unit is missing ownership marker:\n%s", data)
	}
	wantCalls := []string{"daemon-reload", "enable " + unitName}
	if strings.Join(calls, "\n") != strings.Join(wantCalls, "\n") {
		t.Fatalf("unexpected systemctl calls:\n got: %q\nwant: %q", calls, wantCalls)
	}
	for _, call := range calls {
		if strings.Contains(call, "start") {
			t.Fatalf("install unexpectedly started the service: %q", call)
		}
	}
}

func TestLinuxBackendInstallRollbackDoesNotDisableExistingManagedUnit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.Service.Name = "Existing Managed Service"
	unitName := systemdUnitName(cfg.Service.Name)
	unitPath := filepath.Join(dir, unitName)
	previous, err := renderSystemdUnit("/opt/old-codexify-go", "/tmp/old-config.json", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte(previous), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls []string
	b := linuxBackend{
		unitDir: func() (string, error) { return dir, nil },
		systemctl: func(args ...string) (string, error) {
			call := strings.Join(args, " ")
			calls = append(calls, call)
			if call == "enable "+unitName {
				return "enable failed", errors.New("exit status 1")
			}
			return "", nil
		},
	}

	if err := b.install("/opt/new-codexify-go", "/tmp/new-config.json", cfg); err == nil {
		t.Fatal("expected install to fail when enable fails")
	}
	for _, call := range calls {
		if call == "disable "+unitName {
			t.Fatalf("rollback disabled a pre-existing managed unit: %q", calls)
		}
	}
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != previous {
		t.Fatalf("rollback did not restore previous unit:\n%s", data)
	}
}

func TestLinuxBackendStopHandlesTransitionalActiveState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.Service.Name = "Transition Stop"
	unitName := systemdUnitName(cfg.Service.Name)
	unitPath := filepath.Join(dir, unitName)
	data, err := renderSystemdUnit("/opt/codexify-go", "/tmp/config.json", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	stopped := false
	var calls []string
	b := linuxBackend{
		unitDir: func() (string, error) { return dir, nil },
		systemctl: func(args ...string) (string, error) {
			call := strings.Join(args, " ")
			calls = append(calls, call)
			switch {
			case strings.HasPrefix(call, "show "):
				if stopped {
					return "LoadState=loaded\nActiveState=inactive\nSubState=dead\nMainPID=0\nResult=success\n", nil
				}
				return "LoadState=loaded\nActiveState=activating\nSubState=start\nMainPID=321\nResult=success\n", nil
			case call == "stop "+unitName:
				stopped = true
				return "", nil
			default:
				return "", nil
			}
		},
	}

	if err := b.stop(cfg.Service.Name); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatalf("stop was skipped for transitional active state: %q", calls)
	}
}

func TestLinuxBackendRemovePreservesUnitWhenStopFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.Service.Name = "Remove Safety"
	unitName := systemdUnitName(cfg.Service.Name)
	unitPath := filepath.Join(dir, unitName)
	data, err := renderSystemdUnit("/opt/codexify-go", "/tmp/config.json", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	b := linuxBackend{
		unitDir: func() (string, error) { return dir, nil },
		systemctl: func(args ...string) (string, error) {
			switch strings.Join(args, " ") {
			case "show " + unitName + " --property=LoadState --property=ActiveState --property=SubState --property=MainPID --property=Result --no-pager":
				return "LoadState=loaded\nActiveState=active\nSubState=running\nMainPID=123\nResult=success\n", nil
			case "stop " + unitName:
				return "stop failed", errors.New("exit status 1")
			default:
				return "", nil
			}
		},
	}

	if err := b.remove(cfg.Service.Name); err == nil {
		t.Fatal("expected remove to fail when stop fails")
	}
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatalf("unit must remain after failed stop: %v", err)
	}
}
