package secure

import (
	"sync"
	"time"
)

// Limiter allows at most max events per window for each key.
//
// ponytail: in-memory and per process. Use a shared store if the panel runs on
// several instances and the limit must hold across them.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
	now    func() time.Time
}

// NewLimiter returns a limiter allowing max events per window per key.
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, hits: map[string][]time.Time{}, now: time.Now}
}

// Allow records an event for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if len(l.hits) > 10_000 { // bound memory when many distinct keys show up
		for k := range l.hits {
			l.prune(k, now)
		}
	}
	l.prune(key, now)
	if len(l.hits[key]) >= l.max {
		return false
	}
	l.hits[key] = append(l.hits[key], now)
	return true
}

func (l *Limiter) prune(key string, now time.Time) {
	cutoff := now.Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, key)
		return
	}
	l.hits[key] = kept
}
