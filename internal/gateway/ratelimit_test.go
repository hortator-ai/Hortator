/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package gateway

import (
	"net/http"
	"testing"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(5)
	for i := 0; i < 5; i++ {
		if !rl.Allow("client-a") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(3)
	for i := 0; i < 3; i++ {
		rl.Allow("client-a")
	}
	if rl.Allow("client-a") {
		t.Error("4th request should be blocked")
	}
}

func TestRateLimiter_SeparateClients(t *testing.T) {
	rl := NewRateLimiter(2)
	rl.Allow("client-a")
	rl.Allow("client-a")
	// client-a is at limit
	if rl.Allow("client-a") {
		t.Error("client-a should be blocked")
	}
	// client-b should still be allowed
	if !rl.Allow("client-b") {
		t.Error("client-b should be allowed")
	}
}

func TestRateLimiter_DisabledWhenZero(t *testing.T) {
	rl := NewRateLimiter(0)
	for i := 0; i < 100; i++ {
		if !rl.Allow("client") {
			t.Fatal("disabled limiter should always allow")
		}
	}
}

func TestRateLimiter_NilSafe(t *testing.T) {
	var rl *RateLimiter
	if !rl.Allow("key") {
		t.Error("nil limiter should allow")
	}
}

func TestClientKey_BearerToken(t *testing.T) {
	r, _ := http.NewRequest("POST", "/", nil)
	r.Header.Set("Authorization", "Bearer sk-test-123")
	key := ClientKey(r)
	if key != "token:sk-test-123" {
		t.Errorf("key = %q, want token:sk-test-123", key)
	}
}

func TestClientKey_FallsBackToIP(t *testing.T) {
	r, _ := http.NewRequest("POST", "/", nil)
	r.RemoteAddr = "1.2.3.4:12345"
	key := ClientKey(r)
	if key != "ip:1.2.3.4:12345" {
		t.Errorf("key = %q", key)
	}
}

func TestClientKey_XForwardedFor(t *testing.T) {
	r, _ := http.NewRequest("POST", "/", nil)
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	r.RemoteAddr = "1.2.3.4:12345"
	key := ClientKey(r)
	if key != "ip:10.0.0.1" {
		t.Errorf("key = %q, want ip:10.0.0.1", key)
	}
}
