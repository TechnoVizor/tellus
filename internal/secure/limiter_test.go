package secure

import (
	"testing"
	"time"
)

func TestLimiterBlocksAfterMax(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("attempt %d blocked too early", i+1)
		}
	}
	if l.Allow("ip") {
		t.Fatal("4th attempt inside the window was allowed")
	}
	if !l.Allow("other-ip") {
		t.Fatal("limit leaked across keys")
	}
}

func TestLimiterWindowSlides(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	l.Allow("ip")
	l.Allow("ip")
	if l.Allow("ip") {
		t.Fatal("expected block")
	}
	now = now.Add(time.Minute + time.Second)
	if !l.Allow("ip") {
		t.Fatal("still blocked after the window passed")
	}
}
