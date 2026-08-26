package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

// serverListCacheEntry is the cached payload for GET /servers list.

// memCacheEntry holds an in-memory TTL entry for fallback when Redis is absent.
type memCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

type memCache struct {
	mu   sync.RWMutex
	data map[string]*memCacheEntry
}

var globalMemCache = &memCache{data: make(map[string]*memCacheEntry)}

func init() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			globalMemCache.cleanup()
		}
	}()
}

func (m *memCache) get(key string) ([]byte, bool) {
	m.mu.RLock()
	e, ok := m.data[key]
	m.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

func (m *memCache) set(key string, data []byte, ttl time.Duration) {
	m.mu.Lock()
	m.data[key] = &memCacheEntry{data: data, expiresAt: time.Now().Add(ttl)}
	m.mu.Unlock()
}

func (m *memCache) delPrefix(prefix string) {
	m.mu.Lock()
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(m.data, k)
		}
	}
	m.mu.Unlock()
}

func (m *memCache) cleanup() {
	m.mu.Lock()
	now := time.Now()
	for k, e := range m.data {
		if now.After(e.expiresAt) {
			delete(m.data, k)
		}
	}
	m.mu.Unlock()
}

// serversCacheKey builds a bounded cache key for the servers list.
// It includes user identity, pagination, and search so different callers do not share stale data.
func serversCacheKey(userID, role string, page, perPage int, search string) string {
	h := sha256.Sum256([]byte(search))
	return "cache:servers:list:" + userID + ":" + role + ":" + itoaCache(page) + ":" + itoaCache(perPage) + ":" + hex.EncodeToString(h[:8])
}

// allocationsCacheKey for GET /allocations and node allocations.
func allocationsCacheKey(page, perPage int, nodeID string) string {
	if nodeID != "" {
		return "cache:allocations:node:" + nodeID + ":" + itoaCache(page) + ":" + itoaCache(perPage)
	}
	return "cache:allocations:list:" + itoaCache(page) + ":" + itoaCache(perPage)
}

func itoaCache(i int) string {
	return strconv.Itoa(i)
}

// getCachedServers tries Redis then in-memory fallback for servers list.
func getCachedServers(cfg Config, key string) ([]byte, bool) {
	if cfg.Redis != nil && cfg.RedisEnabled {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if data, err := cfg.Redis.Get(ctx, key).Bytes(); err == nil {
			return data, true
		}
	}
	if data, ok := globalMemCache.get(key); ok {
		return data, true
	}
	return nil, false
}

func setCachedServers(cfg Config, key string, data []byte, ttl time.Duration) {
	if cfg.Redis != nil && cfg.RedisEnabled {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = cfg.Redis.Set(ctx, key, data, ttl).Err()
	}
	globalMemCache.set(key, data, ttl)
}

// invalidateServersCache removes all server list caches (called after create/update/delete).
func invalidateServersCache(cfg Config) {
	if cfg.Redis != nil && cfg.RedisEnabled {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		// Use SCAN to avoid blocking Redis; fallback to DEL with pattern via Eval.
		_ = cfg.Redis.Eval(ctx, `for _,k in ipairs(redis.call('keys','cache:servers:list:*')) do redis.call('del',k) end return 1`, []string{}).Err()
	}
	globalMemCache.delPrefix("cache:servers:list:")
}

func invalidateAllocationsCache(cfg Config) {
	if cfg.Redis != nil && cfg.RedisEnabled {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_ = cfg.Redis.Eval(ctx, `for _,k in ipairs(redis.call('keys','cache:allocations:*')) do redis.call('del',k) end return 1`, []string{}).Err()
	}
	globalMemCache.delPrefix("cache:allocations:")
}
