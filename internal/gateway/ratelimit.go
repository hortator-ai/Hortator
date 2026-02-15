/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package gateway

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiter implements per-client sliding window rate limiting using stdlib only.
type RateLimiter struct {
	mu         sync.Mutex
	clients    map[string]*clientWindow
	maxPerMin  int
	windowSize time.Duration
}

// clientWindow tracks request timestamps for a single client.
type clientWindow struct {
	timestamps []time.Time
}

// NewRateLimiter creates a rate limiter with the given requests-per-minute limit.
// If limit <= 0, rate limiting is disabled.
func NewRateLimiter(maxPerMinute int) *RateLimiter {
	return &RateLimiter{
		clients:    make(map[string]*clientWindow),
		maxPerMin:  maxPerMinute,
		windowSize: time.Minute,
	}
}

// DefaultRateLimiter creates a rate limiter using HORTATOR_GATEWAY_RATE_LIMIT
// env var (default: 10 requests/minute).
func DefaultRateLimiter() *RateLimiter {
	limit := 10
	if v := os.Getenv("HORTATOR_GATEWAY_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	return NewRateLimiter(limit)
}

// Allow checks if the client identified by key is within the rate limit.
// Returns true if the request is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	if rl == nil || rl.maxPerMin <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.windowSize)

	cw, ok := rl.clients[key]
	if !ok {
		cw = &clientWindow{}
		rl.clients[key] = cw
	}

	// Prune old timestamps
	valid := cw.timestamps[:0]
	for _, t := range cw.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	cw.timestamps = valid

	if len(cw.timestamps) >= rl.maxPerMin {
		return false
	}

	cw.timestamps = append(cw.timestamps, now)
	return true
}

// ClientKey extracts the rate limit key from an HTTP request.
// Uses the bearer token if present, otherwise falls back to client IP.
func ClientKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return "token:" + strings.TrimPrefix(auth, "Bearer ")
	}
	// Fall back to IP
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.Split(fwd, ",")[0]
		ip = strings.TrimSpace(ip)
	}
	return "ip:" + ip
}
