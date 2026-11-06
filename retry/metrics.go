package retry

import (
	"sync/atomic"
	"time"
)

// Metrics contains optional per-policy cumulative counters; sharing one instance across operations is safe.
type Metrics struct{ attempts, successes, retries, failures, backoff atomic.Int64 }

// Snapshot reports scheduled retries separately from actually executed attempts.
type Snapshot struct {
	Attempts, Successes, ScheduledRetries, Failures int64
	ScheduledBackoff                                time.Duration
}

func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	return Snapshot{m.attempts.Load(), m.successes.Load(), m.retries.Load(), m.failures.Load(), time.Duration(m.backoff.Load())}
}
