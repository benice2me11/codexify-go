package tunnel

import (
	"time"
	"context"
	"testing"
)

func pollFailLine() string {
	return `{"time":"t","level":"WARN","msg":"poll failed; backing off","component":"controlplane","error":"controlplane client: unexpected status 403: unsupported_country_region_territory","tunnel_request_id":"missing_request_id"}` + "\n"
}

func TestPollWatcherCountsConsecutiveFailures(t *testing.T) {
	w := NewPollWatcher(3, 0)
	if err := w.Check(context.Background()); err != nil {
		t.Fatalf("healthy before lines: %v", err)
	}
	if _, err := w.Write([]byte(pollFailLine())); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(context.Background()); err != nil {
		t.Fatalf("below threshold should pass: %v", err)
	}
	w.Write([]byte(pollFailLine()))
	if err := w.Check(context.Background()); err != nil {
		t.Fatalf("second failure still below threshold: %v", err)
	}
	w.Write([]byte(pollFailLine()))
	if err := w.Check(context.Background()); err == nil {
		t.Fatal("third consecutive failure must be unhealthy")
	}
}

func TestPollWatcherResetsOnAliveSignals(t *testing.T) {
	w := NewPollWatcher(2, 0)
	w.Write([]byte(pollFailLine()))
	w.Write([]byte(pollFailLine()))
	w.Write([]byte(`{"msg":"tunnel metadata fetched","component":"controlplane"}` + "\n"))
	if err := w.Check(context.Background()); err != nil {
		t.Fatalf("metadata fetch must reset failures: %v", err)
	}
	w.Write([]byte(pollFailLine()))
	w.Write([]byte(`{"msg":"posted response","component":"controlplane","tunnel_request_id":"abc123"}` + "\n"))
	if err := w.Check(context.Background()); err != nil {
		t.Fatalf("forwarded request must reset failures: %v", err)
	}
}

func TestPollWatcherIgnoresUnrelatedLinesAndPartialWrites(t *testing.T) {
	w := NewPollWatcher(2, 0)
	w.Write([]byte("noise\n"))
	w.Write([]byte(`{"msg":"unrelated"}` + "\n"))
	if err := w.Check(context.Background()); err != nil {
		t.Fatalf("unrelated lines must not trip health: %v", err)
	}
	// Split a failure line across two writes.
	line := pollFailLine()
	w.Write([]byte(line[:len(line)/2]))
	w.Write([]byte(line[len(line)/2:]))
	w.Write([]byte(line))
	if err := w.Check(context.Background()); err == nil {
		t.Fatal("split lines must still be counted")
	}
}

func TestPollWatcherReset(t *testing.T) {
	w := NewPollWatcher(1, 0)
	w.Write([]byte(pollFailLine()))
	if err := w.Check(context.Background()); err == nil {
		t.Fatal("expected unhealthy at threshold 1")
	}
	w.Reset()
	if err := w.Check(context.Background()); err != nil {
		t.Fatalf("reset must clear failures: %v", err)
	}
}

func TestPollWatcherCooldownBoundsRestartRate(t *testing.T) {
	w := NewPollWatcher(1, 60_000_000_000) // 60s cooldown
	fakeNow := time.Now()
	w.now = func() time.Time { return fakeNow }
	w.Write([]byte(pollFailLine()))
	if err := w.Check(context.Background()); err == nil {
		t.Fatal("first trip must be unhealthy")
	}
	w.Reset()
	w.Write([]byte(pollFailLine()))
	if err := w.Check(context.Background()); err != nil {
		t.Fatalf("inside cooldown must stay healthy: %v", err)
	}
	fakeNow = fakeNow.Add(61 * time.Second)
	if err := w.Check(context.Background()); err == nil {
		t.Fatal("after cooldown the next check must trip again")
	}
}
