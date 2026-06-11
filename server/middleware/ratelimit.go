package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimitConfig defines limits per endpoint group.
type RateLimitConfig struct {
	Register   LimitSpec // 3 per hour
	Messages   LimitSpec // 60 per minute
	UserSearch LimitSpec // 30 per minute
	Recovery   LimitSpec // 3 failed -> 1hr lock
}

// LimitSpec defines a rate limit.
type LimitSpec struct {
	MaxRequests int
	Window      time.Duration
	LockOnFail  bool
	LockMax     int
	LockDur     time.Duration
}

// DefaultRateLimitConfig returns sensible defaults.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Register:   LimitSpec{MaxRequests: 3, Window: time.Hour},
		Messages:   LimitSpec{MaxRequests: 60, Window: time.Minute},
		UserSearch: LimitSpec{MaxRequests: 30, Window: time.Minute},
		Recovery:   LimitSpec{MaxRequests: 3, Window: time.Hour, LockOnFail: true, LockMax: 3, LockDur: time.Hour},
	}
}

// Cfg exposes the rate limit configuration.
func (rl *RateLimiter) Cfg() RateLimitConfig { return rl.cfg }

type rateEntry struct {
	count    int
	windowAt time.Time
	locked   bool
	lockedAt time.Time
}

// RateLimiter provides per-endpoint in-memory rate limiting.
type RateLimiter struct {
	mu     sync.Mutex
	entries map[string]*rateEntry
	cfg    RateLimitConfig
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string]*rateEntry),
		cfg:     cfg,
	}
}

// Middleware returns an HTTP middleware that rate-limits based on handler label.
func (rl *RateLimiter) Middleware(label string, spec LimitSpec) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr + ":" + label

			rl.mu.Lock()
			entry, exists := rl.entries[key]
			now := time.Now()

			if !exists {
				entry = &rateEntry{count: 0, windowAt: now}
				rl.entries[key] = entry
			}

			// Reset window if expired
			if now.Sub(entry.windowAt) > spec.Window {
				entry.count = 0
				entry.windowAt = now
			}

			// Check lock
			if entry.locked && spec.LockOnFail {
				if now.Sub(entry.lockedAt) < spec.LockDur {
					rl.mu.Unlock()
					http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
					return
				}
				entry.locked = false
				entry.count = 0
			}

			entry.count++
			if entry.count > spec.MaxRequests {
				if spec.LockOnFail {
					entry.locked = true
					entry.lockedAt = now
				}
				rl.mu.Unlock()
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			rl.mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}
