//go:build windows

package execsession

import (
	"strings"
	"testing"
	"time"
)

func TestQuickCommand(t *testing.T) {
	m := NewManager()
	defer m.Close()
	res, err := m.Start(StartInput{
		Command: "echo hello",
		Shell:   "cmd",
		Yield:   2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Running {
		t.Fatal("expected quick command to finish")
	}
	if res.ExitCode == nil || *res.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %#v", res.ExitCode)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestLongCommandReturnsSessionAndPolls(t *testing.T) {
	m := NewManager()
	defer m.Close()
	res, err := m.Start(StartInput{
		Command: "echo start & ping -n 2 127.0.0.1 >nul & echo done",
		Shell:   "cmd",
		Yield:   50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Running || res.SessionID == "" {
		t.Fatalf("expected running session: %+v", res)
	}
	var output strings.Builder
	output.WriteString(res.Output)
	deadline := time.Now().Add(5 * time.Second)
	for res.Running && time.Now().Before(deadline) {
		res, err = m.Write(res.SessionID, "", 100*time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		output.WriteString(res.Output)
	}
	if res.Running {
		t.Fatal("expected session to finish")
	}
	if !strings.Contains(output.String(), "done") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWriteCtrlCStopsRunningSession(t *testing.T) {
	m := NewManager()
	defer m.Close()
	res, err := m.Start(StartInput{
		Command: "echo ready & ping -t 127.0.0.1 >nul",
		Shell:   "cmd",
		Yield:   50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Running || res.SessionID == "" {
		t.Fatalf("expected running session: %+v", res)
	}

	res, err = m.Write(res.SessionID, "\x03", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Running {
		t.Fatalf("expected Ctrl-C to stop session: %+v", res)
	}
}
