package main

import (
	"strings"
	"sync"
	"time"
)

// IsOriginAllowed checks whether an HTTP Origin header matches the configured allowlist.
func IsOriginAllowed(origin string, allowed []string) bool {
	origin = strings.ToLower(strings.TrimSpace(origin))
	if origin == "" {
		return false
	}
	for _, a := range allowed {
		candidate := strings.ToLower(strings.TrimSpace(a))
		if candidate == "*" || candidate == origin {
			return true
		}
	}
	return false
}

type rateBucket struct {
	tokens     float64
	lastRefill time.Time
}

// IPRateLimiter implements a lightweight per-IP token bucket limiter using stdlib only.
type IPRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	rate    float64 // tokens per second
	burst   float64
}

func NewIPRateLimiter(perMinute int, burst int) *IPRateLimiter {
	if perMinute <= 0 {
		perMinute = 60
	}
	if burst <= 0 {
		burst = perMinute
	}
	return &IPRateLimiter{
		buckets: make(map[string]*rateBucket),
		rate:    float64(perMinute) / 60.0,
		burst:   float64(burst),
	}
}

func (l *IPRateLimiter) Allow(ip string) bool {
	return l.allowAt(ip, time.Now())
}

func (l *IPRateLimiter) allowAt(ip string, now time.Time) bool {
	if ip == "" {
		ip = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) > 4096 {
		cutoff := now.Add(-10 * time.Minute)
		for k, b := range l.buckets {
			if b.lastRefill.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
	}

	b, ok := l.buckets[ip]
	if !ok {
		l.buckets[ip] = &rateBucket{
			tokens:     l.burst - 1,
			lastRefill: now,
		}
		return true
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastRefill = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens -= 1
	return true
}
