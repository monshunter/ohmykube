package utils

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestKeyedSemaphore_BasicAcquireRelease(t *testing.T) {
	ks := NewKeyedSemaphore()
	ctx := context.Background()
	key := "test-key"

	// Test basic acquire and release
	err := ks.Acquire(ctx, key)
	if err != nil {
		t.Fatalf("Failed to acquire semaphore: %v", err)
	}

	// Try to acquire again (should not block since we'll use TryAcquire)
	acquired := ks.TryAcquire(key)
	if acquired {
		t.Error("Expected TryAcquire to fail when semaphore is already held")
	}

	// Release the semaphore
	ks.Release(key)

	// Now TryAcquire should succeed
	acquired = ks.TryAcquire(key)
	if !acquired {
		t.Error("Expected TryAcquire to succeed after release")
	}

	// Release again
	ks.Release(key)
}

func TestKeyedSemaphore_ConcurrentSameKey(t *testing.T) {
	ks := NewKeyedSemaphore()
	key := "concurrent-key"

	var counter int
	var mu sync.Mutex
	var wg sync.WaitGroup

	numGoroutines := 10
	wg.Add(numGoroutines)

	// Start multiple goroutines trying to access the same key
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			ctx := context.Background()
			err := ks.Acquire(ctx, key)
			if err != nil {
				t.Errorf("Goroutine %d: Failed to acquire semaphore: %v", id, err)
				return
			}
			defer ks.Release(key)

			// Critical section - only one goroutine should be here at a time
			mu.Lock()
			localCounter := counter
			time.Sleep(10 * time.Millisecond) // Simulate some work
			counter = localCounter + 1
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// All goroutines should have incremented the counter exactly once
	if counter != numGoroutines {
		t.Errorf("Expected counter to be %d, got %d", numGoroutines, counter)
	}
}

func TestKeyedSemaphore_ConcurrentDifferentKeys(t *testing.T) {
	ks := NewKeyedSemaphore()

	var wg sync.WaitGroup
	numKeys := 5
	wg.Add(numKeys)

	start := time.Now()

	// Start goroutines with different keys (should run in parallel)
	for i := 0; i < numKeys; i++ {
		go func(id int) {
			defer wg.Done()

			key := fmt.Sprintf("key-%d", id)
			ctx := context.Background()

			err := ks.Acquire(ctx, key)
			if err != nil {
				t.Errorf("Goroutine %d: Failed to acquire semaphore: %v", id, err)
				return
			}
			defer ks.Release(key)

			// Simulate work
			time.Sleep(100 * time.Millisecond)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Since different keys should run in parallel, total time should be close to 100ms
	// Allow some margin for scheduling overhead
	if elapsed > 200*time.Millisecond {
		t.Errorf("Expected parallel execution to take ~100ms, took %v", elapsed)
	}
}

func TestKeyedSemaphore_ContextCancellation(t *testing.T) {
	ks := NewKeyedSemaphore()
	key := "cancel-key"

	// First, acquire the semaphore
	ctx1 := context.Background()
	err := ks.Acquire(ctx1, key)
	if err != nil {
		t.Fatalf("Failed to acquire semaphore: %v", err)
	}

	// Create a context that will be cancelled
	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Try to acquire with the cancellable context (should fail)
	start := time.Now()
	err = ks.Acquire(ctx2, key)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected acquire to fail due to context cancellation")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}

	// Should have taken approximately 50ms
	if elapsed < 40*time.Millisecond || elapsed > 100*time.Millisecond {
		t.Errorf("Expected cancellation after ~50ms, took %v", elapsed)
	}

	// Release the first semaphore
	ks.Release(key)
}

func TestGlobalKeyedSemaphore(t *testing.T) {
	key := "global-test-key"
	ctx := context.Background()

	// Test global functions
	err := AcquireKey(ctx, key)
	if err != nil {
		t.Fatalf("Failed to acquire global semaphore: %v", err)
	}

	// Try to acquire again
	acquired := TryAcquireKey(key)
	if acquired {
		t.Error("Expected TryAcquireKey to fail when semaphore is already held")
	}

	// Release
	ReleaseKey(key)

	// Now should be able to acquire
	acquired = TryAcquireKey(key)
	if !acquired {
		t.Error("Expected TryAcquireKey to succeed after release")
	}

	// Release again
	ReleaseKey(key)
}

func TestKeyedSemaphore_SemaphoreReuse(t *testing.T) {
	ks := NewKeyedSemaphore()
	key := "reuse-key"
	ctx := context.Background()

	// Acquire and release multiple times
	for i := 0; i < 5; i++ {
		err := ks.Acquire(ctx, key)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to acquire semaphore: %v", i, err)
		}

		ks.Release(key)
	}

	// Verify the semaphore is still working
	acquired := ks.TryAcquire(key)
	if !acquired {
		t.Error("Expected semaphore to be available after multiple acquire/release cycles")
	}

	ks.Release(key)
}
