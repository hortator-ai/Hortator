/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func TestCacheKey(t *testing.T) {
	k1 := CacheKey("hello world", "researcher")
	k2 := CacheKey("hello world", "researcher")
	k3 := CacheKey("hello world", "tech-lead")
	k4 := CacheKey("different prompt", "researcher")

	if k1 != k2 {
		t.Error("same prompt+role should produce same key")
	}
	if k1 == k3 {
		t.Error("different role should produce different key")
	}
	if k1 == k4 {
		t.Error("different prompt should produce different key")
	}
	if len(k1) != 64 {
		t.Errorf("expected SHA-256 hex (64 chars), got %d chars", len(k1))
	}
}

func TestCacheGetPutBasic(t *testing.T) {
	c := NewResultCache(ResultCacheConfig{Enabled: true, TTL: time.Minute, MaxEntries: 10})

	key := CacheKey("test", "role")

	// Miss
	if c.Get(key) != nil {
		t.Error("expected cache miss")
	}

	// Put
	c.Put(key, &CacheResult{Output: "hello", TokensIn: 10, TokensOut: 20})

	// Hit
	r := c.Get(key)
	if r == nil {
		t.Fatal("expected cache hit")
	}
	if r.Output != "hello" {
		t.Errorf("expected output 'hello', got %q", r.Output)
	}
	if r.TokensIn != 10 || r.TokensOut != 20 {
		t.Errorf("unexpected tokens: %d/%d", r.TokensIn, r.TokensOut)
	}
}

func TestCacheDisabled(t *testing.T) {
	c := NewResultCache(ResultCacheConfig{Enabled: false})

	key := CacheKey("test", "role")
	c.Put(key, &CacheResult{Output: "hello"})

	if c.Get(key) != nil {
		t.Error("disabled cache should always miss")
	}
	if c.Len() != 0 {
		t.Errorf("disabled cache should have 0 entries, got %d", c.Len())
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	c := NewResultCache(ResultCacheConfig{Enabled: true, TTL: 10 * time.Millisecond, MaxEntries: 10})

	key := CacheKey("test", "role")
	c.Put(key, &CacheResult{Output: "hello"})

	// Should hit immediately
	if c.Get(key) == nil {
		t.Fatal("expected cache hit before TTL")
	}

	time.Sleep(20 * time.Millisecond)

	// Should miss after TTL
	if c.Get(key) != nil {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestCacheLRUEviction(t *testing.T) {
	c := NewResultCache(ResultCacheConfig{Enabled: true, TTL: time.Minute, MaxEntries: 3})

	for i := 0; i < 5; i++ {
		key := CacheKey("prompt"+string(rune('A'+i)), "role")
		c.Put(key, &CacheResult{Output: "result"})
	}

	if c.Len() != 3 {
		t.Errorf("expected 3 entries after eviction, got %d", c.Len())
	}

	// First two should be evicted
	if c.Get(CacheKey("promptA", "role")) != nil {
		t.Error("oldest entry should be evicted")
	}
	if c.Get(CacheKey("promptB", "role")) != nil {
		t.Error("second oldest entry should be evicted")
	}
	// Last three should remain
	if c.Get(CacheKey("promptC", "role")) == nil {
		t.Error("entry C should still be cached")
	}
}

func TestCacheDuplicatePut(t *testing.T) {
	c := NewResultCache(ResultCacheConfig{Enabled: true, TTL: time.Minute, MaxEntries: 10})

	key := CacheKey("test", "role")
	c.Put(key, &CacheResult{Output: "first"})
	c.Put(key, &CacheResult{Output: "second"})

	if c.Len() != 1 {
		t.Errorf("duplicate put should not create new entry, got %d", c.Len())
	}
	// Should still return original
	if r := c.Get(key); r.Output != "first" {
		t.Errorf("expected 'first', got %q", r.Output)
	}
}

func TestShouldSkipCache(t *testing.T) {
	tests := []struct {
		name   string
		ann    map[string]string
		expect bool
	}{
		{"no annotations", nil, false},
		{"empty annotations", map[string]string{}, false},
		{"no-cache true", map[string]string{"hortator.ai/no-cache": "true"}, true},
		{"no-cache false", map[string]string{"hortator.ai/no-cache": "false"}, false},
		{"other annotation", map[string]string{"other": "true"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Annotations: tt.ann},
			}
			if got := shouldSkipCache(task); got != tt.expect {
				t.Errorf("shouldSkipCache() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestCacheDefaultConfig(t *testing.T) {
	c := NewResultCache(ResultCacheConfig{Enabled: true})

	if c.config.TTL != 10*time.Minute {
		t.Errorf("expected default TTL 10m, got %v", c.config.TTL)
	}
	if c.config.MaxEntries != 1000 {
		t.Errorf("expected default max entries 1000, got %d", c.config.MaxEntries)
	}
}
