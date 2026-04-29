// Package ratelimit provides simple per-key token-bucket and connection-cap
// limiters. Buckets are created lazily on first use and garbage-collected
// when idle for a configurable retention window.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// TokenBucket enforces a per-key rate (tokens/sec) with a burst size.
type TokenBucket struct {
	mu          sync.Mutex
	rate        float64
	burst       float64
	buckets     map[string]*bucket
	lastSeen    map[string]time.Time
	gcThreshold time.Duration
	now         func() time.Time
}

func New(rate float64, burst float64, gcThreshold time.Duration) *TokenBucket {
	return &TokenBucket{
		rate:        rate,
		burst:       burst,
		buckets:     make(map[string]*bucket),
		lastSeen:    make(map[string]time.Time),
		gcThreshold: gcThreshold,
		now:         time.Now,
	}
}

// SetNowFn is a hook for tests.
func (t *TokenBucket) SetNowFn(fn func() time.Time) { t.now = fn }

// Allow returns true if the key has at least 1 token available; consumes
// 1 token on success.
func (t *TokenBucket) Allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	b, ok := t.buckets[key]
	if !ok {
		b = &bucket{tokens: t.burst, lastRefill: now}
		t.buckets[key] = b
	}
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * t.rate
		if b.tokens > t.burst {
			b.tokens = t.burst
		}
		b.lastRefill = now
	}
	t.lastSeen[key] = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// SweepIdle drops buckets that haven't been touched in gcThreshold.
func (t *TokenBucket) SweepIdle() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for k, ts := range t.lastSeen {
		if now.Sub(ts) > t.gcThreshold {
			delete(t.buckets, k)
			delete(t.lastSeen, k)
		}
	}
}

// ConnCap caps simultaneous connections per key.
type ConnCap struct {
	mu     sync.Mutex
	max    int
	counts map[string]int
}

func NewConnCap(max int) *ConnCap {
	return &ConnCap{max: max, counts: make(map[string]int)}
}

func (c *ConnCap) Acquire(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts[key] >= c.max {
		return false
	}
	c.counts[key]++
	return true
}

func (c *ConnCap) Release(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts[key] > 0 {
		c.counts[key]--
		if c.counts[key] == 0 {
			delete(c.counts, key)
		}
	}
}
