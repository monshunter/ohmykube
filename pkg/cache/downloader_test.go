package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDownloadFile_ThreadSafety tests that concurrent downloads of the same file are handled correctly
func TestDownloadFile_ThreadSafety(t *testing.T) {
	// Track how many times the server is actually hit
	var serverHitCount int64

	// Create a test server that simulates a slow download
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&serverHitCount, 1)
		// Simulate a slow download
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "test content")
	}))
	defer server.Close()

	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "downloader_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	downloader := NewDownloader()
	url := server.URL
	destPath := filepath.Join(tempDir, "test_file.txt")

	// Track successful downloads
	var successCount int64
	var wg sync.WaitGroup

	// Start multiple concurrent downloads of the same file
	numGoroutines := 5
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			ctx := context.Background()
			err := downloader.DownloadFile(ctx, url, destPath)
			if err != nil {
				t.Errorf("Goroutine %d: Download failed: %v", id, err)
				return
			}

			atomic.AddInt64(&successCount, 1)

			// Check if file exists and has correct content
			content, err := os.ReadFile(destPath)
			if err != nil {
				t.Errorf("Goroutine %d: Failed to read downloaded file: %v", id, err)
				return
			}

			if string(content) != "test content" {
				t.Errorf("Goroutine %d: File content mismatch. Expected 'test content', got '%s'", id, string(content))
			}
		}(i)
	}

	wg.Wait()

	// Verify that all goroutines completed successfully
	finalSuccessCount := atomic.LoadInt64(&successCount)
	if finalSuccessCount != int64(numGoroutines) {
		t.Errorf("Expected %d successful downloads, got %d", numGoroutines, finalSuccessCount)
	}

	// Most importantly: verify that the server was only hit once
	// This proves that the semaphore mechanism is working correctly
	finalServerHits := atomic.LoadInt64(&serverHitCount)
	if finalServerHits != 1 {
		t.Errorf("Expected server to be hit only once, but was hit %d times", finalServerHits)
	}

	// Verify that the file exists and has correct content
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("Downloaded file does not exist")
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read final file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("Final file content mismatch. Expected 'test content', got '%s'", string(content))
	}
}

// TestDownloadFile_DifferentFiles tests that downloads of different files can proceed concurrently
func TestDownloadFile_DifferentFiles(t *testing.T) {
	var serverHitCount int64

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&serverHitCount, 1)
		// Simulate different content based on URL path
		path := r.URL.Path
		time.Sleep(50 * time.Millisecond) // Small delay
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, fmt.Sprintf("content for %s", path))
	}))
	defer server.Close()

	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "downloader_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	downloader := NewDownloader()

	var wg sync.WaitGroup
	numFiles := 3
	wg.Add(numFiles)

	start := time.Now()

	// Download different files concurrently
	for i := 0; i < numFiles; i++ {
		go func(id int) {
			defer wg.Done()

			url := fmt.Sprintf("%s/file%d", server.URL, id)
			destPath := filepath.Join(tempDir, fmt.Sprintf("file%d.txt", id))

			ctx := context.Background()
			err := downloader.DownloadFile(ctx, url, destPath)
			if err != nil {
				t.Errorf("Download of file%d failed: %v", id, err)
				return
			}

			// Verify content
			content, err := os.ReadFile(destPath)
			if err != nil {
				t.Errorf("Failed to read file%d: %v", id, err)
				return
			}

			expected := fmt.Sprintf("content for /file%d", id)
			if string(content) != expected {
				t.Errorf("File%d content mismatch. Expected '%s', got '%s'", id, expected, string(content))
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Verify all files were downloaded (server should be hit once per file)
	finalServerHits := atomic.LoadInt64(&serverHitCount)
	if finalServerHits != int64(numFiles) {
		t.Errorf("Expected server to be hit %d times, but was hit %d times", numFiles, finalServerHits)
	}

	// Since different files should run in parallel, total time should be close to 50ms
	// Allow some margin for scheduling overhead
	if elapsed > 150*time.Millisecond {
		t.Errorf("Expected parallel execution to take ~50ms, took %v", elapsed)
	}

	// Verify all files exist
	for i := 0; i < numFiles; i++ {
		destPath := filepath.Join(tempDir, fmt.Sprintf("file%d.txt", i))
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("File%d does not exist", i)
		}
	}
}

// TestDownloadFile_ContextCancellation tests that context cancellation works correctly
func TestDownloadFile_ContextCancellation(t *testing.T) {
	// Create a test server that simulates a very slow download
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Very slow
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "test content")
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "downloader_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	downloader := NewDownloader()
	url := server.URL
	destPath := filepath.Join(tempDir, "test_file.txt")

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = downloader.DownloadFile(ctx, url, destPath)
	if err == nil {
		t.Error("Expected download to fail due to context cancellation, but it succeeded")
	}

	// Check if the error is related to context cancellation
	if ctx.Err() == nil {
		t.Error("Expected context to be cancelled")
	}
}

// TestKeyedSemaphore_Integration tests the integration between downloader and keyed semaphore
func TestKeyedSemaphore_Integration(t *testing.T) {
	// This test verifies that the keyed semaphore mechanism works correctly
	// when integrated with the downloader

	var serverHitCount int64

	// Create a test server that simulates a slow download
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&serverHitCount, 1)
		// Simulate a slow download to increase chance of race conditions
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "integration test content")
	}))
	defer server.Close()

	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "integration_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	downloader := NewDownloader()
	url := server.URL
	destPath := filepath.Join(tempDir, "integration_test.txt")

	// Start many concurrent downloads to stress test the semaphore
	numGoroutines := 10
	var wg sync.WaitGroup
	var successCount int64

	wg.Add(numGoroutines)
	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			ctx := context.Background()
			err := downloader.DownloadFile(ctx, url, destPath)
			if err != nil {
				t.Errorf("Goroutine %d: Download failed: %v", id, err)
				return
			}

			atomic.AddInt64(&successCount, 1)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Verify all downloads succeeded
	finalSuccessCount := atomic.LoadInt64(&successCount)
	if finalSuccessCount != int64(numGoroutines) {
		t.Errorf("Expected %d successful downloads, got %d", numGoroutines, finalSuccessCount)
	}

	// Most importantly: verify that the server was only hit once
	finalServerHits := atomic.LoadInt64(&serverHitCount)
	if finalServerHits != 1 {
		t.Errorf("Expected server to be hit only once, but was hit %d times", finalServerHits)
	}

	// The total time should be close to the single download time (200ms)
	// since only one actual download should occur
	if elapsed > 500*time.Millisecond {
		t.Errorf("Expected execution time ~200ms (single download), took %v", elapsed)
	}

	// Verify file content
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(content) != "integration test content" {
		t.Errorf("File content mismatch. Expected 'integration test content', got '%s'", string(content))
	}
}
