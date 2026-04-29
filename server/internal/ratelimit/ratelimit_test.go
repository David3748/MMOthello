package ratelimit

import (
	"testing"
	"time"
)

func TestTokenBucketRefills(t *testing.T) {
	tb := New(1.0, 2.0, time.Hour)
	now := time.Unix(0, 0)
	tb.SetNowFn(func() time.Time { return now })

	if !tb.Allow("a") || !tb.Allow("a") {
		t.Fatalf("expected to consume initial burst")
	}
	if tb.Allow("a") {
		t.Fatalf("third request before refill should be denied")
	}
	now = now.Add(1500 * time.Millisecond)
	if !tb.Allow("a") {
		t.Fatalf("expected refill at 1.5s for rate=1/s")
	}
}

func TestTokenBucketIsolatesKeys(t *testing.T) {
	tb := New(1.0, 1.0, time.Hour)
	now := time.Unix(0, 0)
	tb.SetNowFn(func() time.Time { return now })

	if !tb.Allow("a") {
		t.Fatalf("a should be allowed once")
	}
	if !tb.Allow("b") {
		t.Fatalf("b should not be affected by a")
	}
}

func TestConnCap(t *testing.T) {
	cap := NewConnCap(2)
	if !cap.Acquire("k") || !cap.Acquire("k") {
		t.Fatalf("first two acquires should succeed")
	}
	if cap.Acquire("k") {
		t.Fatalf("third acquire should fail")
	}
	cap.Release("k")
	if !cap.Acquire("k") {
		t.Fatalf("acquire after release should succeed")
	}
}

func TestSweepIdle(t *testing.T) {
	tb := New(1.0, 1.0, time.Second)
	now := time.Unix(0, 0)
	tb.SetNowFn(func() time.Time { return now })
	_ = tb.Allow("a")
	now = now.Add(2 * time.Second)
	tb.SweepIdle()
	tb.mu.Lock()
	_, ok := tb.buckets["a"]
	tb.mu.Unlock()
	if ok {
		t.Fatalf("expected idle bucket to be swept")
	}
}
