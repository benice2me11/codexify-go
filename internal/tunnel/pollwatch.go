package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PollWatcher taps the tunnel child's JSON log stream and reports the
// control-plane poller as unhealthy after too many consecutive poll failures.
// The local /readyz endpoint stays green while the poller keeps dead egress
// (for example after a VPN interface flap), so the log stream is the only
// place the failure is observable from the supervising process.
type PollWatcher struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	now       func() time.Time
	lastTrip  time.Time
	tripped   bool
	failures  int
	buf       []byte
}

func NewPollWatcher(threshold int, cooldown time.Duration) *PollWatcher {
	if threshold < 1 {
		threshold = 1
	}
	if cooldown <= 0 {
		cooldown = 6 * time.Minute
	}
	return &PollWatcher{threshold: threshold, cooldown: cooldown, now: time.Now}
}

// Reset clears the failure count; call when a new child process starts.
func (w *PollWatcher) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failures = 0
	w.tripped = false
	w.buf = w.buf[:0]
}

func (w *PollWatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		idx := -1
		for i, b := range w.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := w.buf[:idx]
		w.buf = w.buf[idx+1:]
		w.scan(line)
	}
	return len(p), nil
}

func (w *PollWatcher) scan(line []byte) {
	var rec struct {
		Msg       string `json:"msg"`
		Component string `json:"component"`
		RequestID string `json:"tunnel_request_id"`
	}
	if err := json.Unmarshal(line, &rec); err != nil {
		return
	}
	switch {
	case rec.Component == "controlplane" && strings.HasPrefix(rec.Msg, "poll failed"):
		w.failures++
	case rec.Msg == "tunnel metadata fetched" || strings.Contains(rec.Msg, "session connected"):
		w.failures = 0
		w.tripped = false
	case rec.RequestID != "" && rec.RequestID != "missing_request_id":
		w.failures = 0
		w.tripped = false
	}
}

// Check implements supervisor.Checker. Once tripped it keeps failing on
// every check until the supervisor restarts the child (Reset) or the poller
// recovers on its own, so the supervisor sees enough consecutive failures to
// act. The cooldown only suppresses a *new* trip after a restart, bounding
// the restart rate during a sustained outage instead of causing churn.
func (w *PollWatcher) Check(context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.tripped {
		return fmt.Errorf("control-plane poll failed %d consecutive times", w.failures)
	}
	if w.failures < w.threshold {
		return nil
	}
	if !w.lastTrip.IsZero() && w.now().Sub(w.lastTrip) < w.cooldown {
		return nil
	}
	w.tripped = true
	w.lastTrip = w.now()
	return fmt.Errorf("control-plane poll failed %d consecutive times", w.failures)
}
