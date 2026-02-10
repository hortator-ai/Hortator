package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

// ResultCacheConfig holds configuration for the result cache.
type ResultCacheConfig struct {
	Enabled    bool
	TTL        time.Duration
	MaxEntries int
}

// cacheEntry stores a cached task result.
type cacheEntry struct {
	output    string
	tokensIn  int64
	tokensOut int64
	model     string
	cachedAt  time.Time
}

// ResultCache provides content-addressable result caching keyed on prompt+role hash.
// This avoids spawning duplicate Pods for identical tasks. Cache hits return instantly
// without creating any K8s resources.
//
// Design decisions:
//   - One-shot cache: keyed on exact prompt+role (no fuzzy matching)
//   - In-memory only: restarts clear cache (acceptable — cache is pure optimization)
//   - LRU eviction: when MaxEntries exceeded, oldest entry evicted
//   - Opt-out: tasks with annotation "hortator.ai/no-cache=true" bypass cache
//   - Only successful results are cached (no caching of failures)
type ResultCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	order   []string // insertion order for LRU eviction
	config  ResultCacheConfig
}

// NewResultCache creates a new result cache with the given configuration.
func NewResultCache(cfg ResultCacheConfig) *ResultCache {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 1000
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 10 * time.Minute
	}
	return &ResultCache{
		entries: make(map[string]*cacheEntry, cfg.MaxEntries),
		config:  cfg,
	}
}

// CacheKey computes a SHA-256 hash of prompt+role for cache lookup.
func CacheKey(prompt, role string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(role))
	_, _ = h.Write([]byte{0}) // separator
	_, _ = h.Write([]byte(prompt))
	return hex.EncodeToString(h.Sum(nil))
}

// CacheResult holds the data returned on a cache hit.
type CacheResult struct {
	Output    string
	TokensIn  int64
	TokensOut int64
	Model     string
}

// Get looks up a cached result by key. Returns nil if not found or expired.
func (c *ResultCache) Get(key string) *CacheResult {
	if !c.config.Enabled {
		return nil
	}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil
	}

	if time.Since(entry.cachedAt) > c.config.TTL {
		// Expired — remove lazily
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil
	}

	return &CacheResult{
		Output:    entry.output,
		TokensIn:  entry.tokensIn,
		TokensOut: entry.tokensOut,
		Model:     entry.model,
	}
}

// Put stores a result in the cache. Evicts oldest entry if at capacity.
func (c *ResultCache) Put(key string, result *CacheResult) {
	if !c.config.Enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Don't store duplicates
	if _, exists := c.entries[key]; exists {
		return
	}

	// Evict oldest if at capacity
	for len(c.entries) >= c.config.MaxEntries {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		output:    result.Output,
		tokensIn:  result.TokensIn,
		tokensOut: result.TokensOut,
		model:     result.Model,
		cachedAt:  time.Now(),
	}
	c.order = append(c.order, key)
}

// evictOldest removes the oldest entry. Must be called with mu held.
func (c *ResultCache) evictOldest() {
	for len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		if _, exists := c.entries[oldest]; exists {
			delete(c.entries, oldest)
			return
		}
		// Key already removed (e.g. expired), skip
	}
}

// Len returns the current number of entries.
func (c *ResultCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// shouldSkipCache returns true if the task has opted out of caching.
func shouldSkipCache(task *corev1alpha1.AgentTask) bool {
	if task.Annotations == nil {
		return false
	}
	return task.Annotations["hortator.ai/no-cache"] == "true"
}
