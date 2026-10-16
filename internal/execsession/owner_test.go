package execsession

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOwnedSessionRestrictsStdinAndStop(t *testing.T) {
	m := NewManager()
	defer m.Close()

	cmd, shell := "echo start; sleep 2; echo done", "sh"
	if runtime.GOOS == "windows" {
		cmd, shell = "echo start & ping -n 6 127.0.0.1 >nul & echo done", "cmd"
	}
	owner := Owner{
		Conversation: "chat-a",
		Group:        "job-search",
		Repository:   "hh",
		Root:         "fixture-hh",
		Fingerprint:  "snapshot-1",
		BindingEpoch: "first-binding",
	}
	started, err := m.Start(StartInput{
		Command: cmd, Shell: shell, Yield: 35 * time.Millisecond, Owner: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !started.Running || started.SessionID == "" {
		t.Fatalf("expected long-running fixture session: %+v", started)
	}
	id := started.SessionID
	if _, err := m.OwnerFor(id, "chat-b"); err == nil {
		t.Fatal("foreign conversation accessed owner metadata")
	}
	if _, err := m.OwnerFor(id, "chat-a"); err != nil {
		t.Fatalf("original conversation must access own owner: %v", err)
	}
	if _, err := m.Write(id, "", 20*time.Millisecond); err == nil {
		t.Fatal("unscoped write to owned session succeeded")
	}
	if err := m.Stop(context.Background(), id); err == nil {
		t.Fatal("unscoped stop on owned session succeeded")
	}
	for _, other := range []Owner{
		{Conversation: "chat-b", Group: "job-search", Repository: "hh", Root: "fixture-hh", Fingerprint: "snapshot-1", BindingEpoch: "first-binding"},
		{Conversation: "chat-a", Group: "job-search", Repository: "habr", Root: "fixture-hh", Fingerprint: "snapshot-1", BindingEpoch: "first-binding"},
		{Conversation: "chat-a", Group: "job-search", Repository: "hh", Root: "fixture-habr", Fingerprint: "snapshot-1", BindingEpoch: "first-binding"},
		{Conversation: "chat-a", Group: "job-search", Repository: "hh", Root: "fixture-hh", Fingerprint: "different", BindingEpoch: "first-binding"},
		{Conversation: "chat-a", Group: "job-search", Repository: "hh", Root: "fixture-hh", Fingerprint: "snapshot-1", BindingEpoch: "later-binding"},
		{},
	} {
		if _, err := m.WriteOwned(id, other, "", 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "foreign") {
			t.Fatalf("modified owner %+v was able to write: %v", other, err)
		}
		if err := m.StopOwned(context.Background(), id, other); err == nil {
			t.Fatalf("modified owner %+v was able to stop session", other)
		}
	}
	_, err = m.WriteOwned(id, owner, "", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("original conversation/checkout cannot poll: %v", err)
	}
	if err := m.StopOwned(context.Background(), id, owner); err != nil {
		t.Fatalf("owner could not stop session: %v", err)
	}
}
