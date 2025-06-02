package utils

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

// KeyedSemaphore provides per-key semaphore functionality
// It ensures that operations with the same key are serialized
type KeyedSemaphore struct {
	semaphores sync.Map // map[string]*semaphore.Weighted
}

// NewKeyedSemaphore creates a new keyed semaphore manager
func NewKeyedSemaphore() *KeyedSemaphore {
	return &KeyedSemaphore{}
}

// getSemaphore gets or creates a semaphore for the given key
func (ks *KeyedSemaphore) getSemaphore(key string) *semaphore.Weighted {
	// Try to load existing semaphore
	if sem, exists := ks.semaphores.Load(key); exists {
		return sem.(*semaphore.Weighted)
	}

	// Create new semaphore with weight 1 (binary semaphore)
	newSem := semaphore.NewWeighted(1)

	// Store it atomically (LoadOrStore returns the actual value that was stored)
	if actualSem, loaded := ks.semaphores.LoadOrStore(key, newSem); loaded {
		// Another goroutine created the semaphore first, use that one
		return actualSem.(*semaphore.Weighted)
	}

	// We successfully stored our semaphore
	return newSem
}

// Acquire acquires the semaphore for the given key
// It blocks until the semaphore is available or the context is cancelled
func (ks *KeyedSemaphore) Acquire(ctx context.Context, key string) error {
	sem := ks.getSemaphore(key)
	return sem.Acquire(ctx, 1)
}

// Release releases the semaphore for the given key
func (ks *KeyedSemaphore) Release(key string) {
	sem := ks.getSemaphore(key)
	sem.Release(1)
}

// TryAcquire tries to acquire the semaphore for the given key without blocking
// Returns true if acquired, false if not available
func (ks *KeyedSemaphore) TryAcquire(key string) bool {
	sem := ks.getSemaphore(key)
	return sem.TryAcquire(1)
}

// Global instance for package-level convenience functions
var globalKeyedSemaphore = NewKeyedSemaphore()

// AcquireKey acquires the global semaphore for the given key
func AcquireKey(ctx context.Context, key string) error {
	return globalKeyedSemaphore.Acquire(ctx, key)
}

// ReleaseKey releases the global semaphore for the given key
func ReleaseKey(key string) {
	globalKeyedSemaphore.Release(key)
}

// TryAcquireKey tries to acquire the global semaphore for the given key without blocking
func TryAcquireKey(key string) bool {
	return globalKeyedSemaphore.TryAcquire(key)
}
