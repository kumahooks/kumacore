package cache_test

import (
	"sync"
	"testing"
	"time"

	"kumacore/core/cache"
)

func TestNewCache_ZeroTTL_NeverExpires(t *testing.T) {
	cacheInstance := cache.NewCache[string, string](0)
	cacheInstance.Set("lain", "god")

	got, ok := cacheInstance.Get("lain")
	if !ok {
		t.Fatal("Get: got miss, want hit for zero-TTL entry")
	}

	if got != "god" {
		t.Errorf("Get: got %q, want %q", got, "god")
	}
}

func TestGet_Hit_ReturnsValue(t *testing.T) {
	cacheInstance := cache.NewCache[string, int](time.Hour)
	cacheInstance.Set("lain", 42)

	got, ok := cacheInstance.Get("lain")
	if !ok {
		t.Fatal("Get: got miss, want hit")
	}

	if got != 42 {
		t.Errorf("Get: got %d, want %d", got, 42)
	}
}

func TestGet_Miss_ReturnsFalse(t *testing.T) {
	cacheInstance := cache.NewCache[string, int](time.Hour)

	_, ok := cacheInstance.Get("lain?")
	if ok {
		t.Error("Get: got hit, want miss for non-existent key")
	}
}

func TestGet_ExpiredEntry_ReturnsMissAndRemovesEntry(t *testing.T) {
	cacheInstance := cache.NewCache[string, string](50 * time.Millisecond)
	cacheInstance.Set("lain", "is gone")

	time.Sleep(60 * time.Millisecond)

	_, ok := cacheInstance.Get("lain")
	if ok {
		t.Error("Get: got hit, want miss for expired entry")
	}

	if cacheInstance.Len() != 0 {
		t.Errorf("Len after expired Get: got %d, want 0", cacheInstance.Len())
	}
}

func TestGet_ExpiredEntry_WithZeroTTL_DoesNotExpire(t *testing.T) {
	cacheInstance := cache.NewCache[string, string](0)
	cacheInstance.Set("lain", "is forever")

	time.Sleep(20 * time.Millisecond)

	got, ok := cacheInstance.Get("lain")
	if !ok {
		t.Fatal("Get: got miss, want hit for zero-TTL entry after sleep")
	}

	if got != "is forever" {
		t.Errorf("Get: got %q, want %q", got, "is forever")
	}
}

func TestSet_OverwritesExistingEntry(t *testing.T) {
	cacheInstance := cache.NewCache[string, string](time.Hour)

	cacheInstance.Set("lain", "first")
	cacheInstance.Set("lain", "second")

	got, ok := cacheInstance.Get("lain")
	if !ok {
		t.Fatal("Get: got miss, want hit")
	}

	if got != "second" {
		t.Errorf("Get: got %q, want %q (overwritten value)", got, "second")
	}
}

func TestRemove_DeletesEntry(t *testing.T) {
	cacheInstance := cache.NewCache[string, string](time.Hour)

	cacheInstance.Set("lain", "owo")
	cacheInstance.Remove("lain")

	_, ok := cacheInstance.Get("lain")
	if ok {
		t.Error("Get after Remove: got hit, want miss")
	}
}

func TestRemove_NonExistentKey_NoError(t *testing.T) {
	cacheInstance := cache.NewCache[string, string](time.Hour)
	cacheInstance.Remove("lain")
}

func TestSet_AfterRemove_ReAddsEntry(t *testing.T) {
	cacheInstance := cache.NewCache[string, string](time.Hour)

	cacheInstance.Set("lain", "first")
	cacheInstance.Remove("lain")
	cacheInstance.Set("lain", "second")

	got, ok := cacheInstance.Get("lain")
	if !ok {
		t.Fatal("Get after re-add: got miss, want hit")
	}

	if got != "second" {
		t.Errorf("Get after re-add: got %q, want %q", got, "second")
	}
}

func TestPurge_RemovesAllEntries(t *testing.T) {
	cacheInstance := cache.NewCache[string, int](time.Hour)

	cacheInstance.Set("lain", 1)
	cacheInstance.Set("is", 2)
	cacheInstance.Set("god", 3)

	cacheInstance.Purge()

	if cacheInstance.Len() != 0 {
		t.Errorf("Len after Purge: got %d, want 0", cacheInstance.Len())
	}

	_, ok := cacheInstance.Get("a")
	if ok {
		t.Error("Get after Purge: got hit, want miss")
	}
}

func TestLen_ReflectsEntryCount(t *testing.T) {
	cacheInstance := cache.NewCache[string, int](time.Hour)

	if cacheInstance.Len() != 0 {
		t.Fatalf("Len: got %d, want 0", cacheInstance.Len())
	}

	cacheInstance.Set("lain", 1)
	cacheInstance.Set("wired", 2)

	if cacheInstance.Len() != 2 {
		t.Errorf("Len: got %d, want 2", cacheInstance.Len())
	}

	cacheInstance.Remove("wired")

	if cacheInstance.Len() != 1 {
		t.Errorf("Len after Remove: got %d, want 1", cacheInstance.Len())
	}
}

func TestGet_ConcurrentAccess_IsSafe(t *testing.T) {
	cacheInstance := cache.NewCache[int, int](time.Hour)

	for index := range 100 {
		cacheInstance.Set(index, index*2)
	}

	var waitGroup sync.WaitGroup

	for range 10 {
		waitGroup.Go(func() {
			for index := range 100 {
				cacheInstance.Get(index)
				cacheInstance.Set(index+100, index)
				cacheInstance.Remove(index + 100)
			}
		})
	}

	waitGroup.Wait()

	// Verify the initial entries survive concurrent access.
	for index := range 100 {
		got, ok := cacheInstance.Get(index)
		if !ok {
			t.Errorf("key %d missing after concurrent access", index)
			continue
		}

		if got != index*2 {
			t.Errorf("key %d: got %d, want %d", index, got, index*2)
		}
	}
}

// TestGet_ExpiredEntry_RacingWithSet_PreservesFreshEntry exercises the
// double-check locking path in Get: an entry is expired under the RLock, but
// a concurrent Set refreshes it before the write lock is acquired. The fresh
// entry must survive.
func TestGet_ExpiredEntry_RacingWithSet_PreservesFreshEntry(t *testing.T) {
	cacheInstance := cache.NewCache[string, int](10 * time.Millisecond)

	cacheInstance.Set("key", 1)

	time.Sleep(15 * time.Millisecond)

	var waitGroup sync.WaitGroup

	// Reader sees an expired entry and enters the write-lock path to delete.
	waitGroup.Go(func() {
		cacheInstance.Get("key")
	})

	// Concurrent writer refreshes the entry before the reader's delete.
	waitGroup.Go(func() {
		cacheInstance.Set("key", 2)
	})

	waitGroup.Wait()

	got, ok := cacheInstance.Get("key")
	if !ok {
		t.Fatal("Get: fresh entry was deleted by racing expired Get")
	}

	if got != 2 {
		t.Errorf("Get: got %d, want 2", got)
	}
}
