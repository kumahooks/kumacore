// Package cache provides a generic TTL-based in-memory cache.
//
// Entries expire lazily on Get: if an entry's expiration time has passed it is
// treated as a miss and removed from the map. There is no background cleanup
// goroutine, so expired entries that are never read again remain in memory
// until explicit Purge or process exit.
package cache

import (
	"sync"
	"time"
)

// cacheEntry holds a cached value and its expiration timestamp.
type cacheEntry[V any] struct {
	value     V
	expiresAt time.Time
}

// Cache is a map of keys to TTL-stamped values.
// A zero TTL means entries never expire.
type Cache[K comparable, V any] struct {
	mutex   sync.RWMutex
	ttl     time.Duration
	entries map[K]cacheEntry[V]
}

// NewCache returns a Cache whose entries expire after ttl.
// If ttl is zero, entries never expire.
func NewCache[K comparable, V any](ttl time.Duration) *Cache[K, V] {
	return &Cache[K, V]{
		ttl:     ttl,
		entries: make(map[K]cacheEntry[V]),
	}
}

// Get returns the cached value for key and true if it exists and has not expired.
// Expired entries are removed lazily during this call.
func (cache *Cache[K, V]) Get(key K) (V, bool) {
	cache.mutex.RLock()
	cached, ok := cache.entries[key]
	cache.mutex.RUnlock()

	if !ok {
		var zero V
		return zero, false
	}

	if cache.ttl > 0 && time.Now().After(cached.expiresAt) {
		cache.mutex.Lock()
		// Re-check under write lock to avoid double-delete races.
		if current, stillExists := cache.entries[key]; stillExists && time.Now().After(current.expiresAt) {
			delete(cache.entries, key)
		}
		cache.mutex.Unlock()

		var zero V
		return zero, false
	}

	return cached.value, true
}

// Set stores value under key with the cache's TTL applied from now.
func (cache *Cache[K, V]) Set(key K, value V) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	var expiresAt time.Time
	if cache.ttl > 0 {
		expiresAt = time.Now().Add(cache.ttl)
	}

	cache.entries[key] = cacheEntry[V]{value: value, expiresAt: expiresAt}
}

// Remove deletes the entry for key if it exists.
func (cache *Cache[K, V]) Remove(key K) {
	cache.mutex.Lock()
	delete(cache.entries, key)
	cache.mutex.Unlock()
}

// Purge removes all entries from the cache.
func (cache *Cache[K, V]) Purge() {
	cache.mutex.Lock()
	cache.entries = make(map[K]cacheEntry[V])
	cache.mutex.Unlock()
}

// Len returns the number of entries currently in the map, including expired
// entries that have not been lazily removed yet.
func (cache *Cache[K, V]) Len() int {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()

	return len(cache.entries)
}
