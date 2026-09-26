package secure

import (
	"testing"
	"time"
)

func TestLimiterResetForgetsOnlyThatKey(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(1, time.Minute)
	l.now = func() time.Time { return now }

	l.Allow("a")
	l.Allow("b")
	if l.Allow("a") || l.Allow("b") {
		t.Fatal("setup: both keys should be at their limit")
	}

	l.Reset("a")
	if !l.Allow("a") {
		t.Error("Reset must give the key a fresh allowance")
	}
	if l.Allow("b") {
		t.Error("Reset leaked to another key")
	}
}
