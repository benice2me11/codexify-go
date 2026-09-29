//go:build darwin

package service

import (
	"strings"
	"testing"
)

func TestLaunchdLabelSanitizesServiceName(t *testing.T) {
	got := launchdLabel("Codexify Go / Dev")
	if got != "io.github.benice2me11.codexify-go.codexify-go---dev" {
		t.Fatalf("label=%q", got)
	}
}

func TestLaunchdLabelFallsBackWhenSanitizedNameIsEmpty(t *testing.T) {
	got := launchdLabel("///")
	if got != "io.github.benice2me11.codexify-go.service" {
		t.Fatalf("label=%q", got)
	}
}

func TestLaunchdPlistContainsLifecycleKeys(t *testing.T) {
	text := plistXML(launchdPlist{
		Label:             "io.github.benice2me11.codexify-go.test",
		ProgramArguments:  []string{"/tmp/codexify-go", "service", "run", "--config", "/tmp/config.json"},
		RunAtLoad:         true,
		KeepAlive:         launchdKeepAlive{SuccessfulExit: boolPtr(false)},
		ProcessType:       "Background",
		ThrottleInterval:  5,
		StandardOutPath:   "/tmp/stdout.log",
		StandardErrorPath: "/tmp/stderr.log",
	})
	for _, want := range []string{
		"<key>Label</key>",
		"<string>io.github.benice2me11.codexify-go.test</string>",
		"<key>ProgramArguments</key>",
		"<string>service</string>",
		"<key>RunAtLoad</key>",
		"<true/>",
		"<key>KeepAlive</key>",
		"<key>SuccessfulExit</key>",
		"<false/>",
		"<key>ProcessType</key>",
		"<string>Background</string>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("plist missing %q:\n%s", want, text)
		}
	}
}

func TestParseLaunchctlPID(t *testing.T) {
	out := "{\n\tstate = running\n\tpid = 4242\n}\n"
	if got := parseLaunchctlPID(out); got != 4242 {
		t.Fatalf("pid=%d", got)
	}
	if got := parseLaunchctlPID("state = waiting\n"); got != 0 {
		t.Fatalf("pid=%d", got)
	}
}
