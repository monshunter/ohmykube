package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/monshunter/ohmykube/pkg/log"
)

// Downloader handles downloading packages from remote URLs
type Downloader struct {
	client *http.Client
}

// NewDownloader creates a new downloader instance
func NewDownloader() *Downloader {
	return &Downloader{
		client: &http.Client{
			Timeout: 30 * time.Minute, // Allow long downloads
		},
	}
}

// DownloadFile downloads a file from the given URL to the specified destination
func (d *Downloader) DownloadFile(ctx context.Context, url, destPath string) error {
	log.Infof("Downloading %s to %s", url, destPath)

	// Create the destination directory if it doesn't exist
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", destDir, err)
	}

	// Create the request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "ohmykube/1.0")

	// Execute the request
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download from %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	// Create the destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", destPath, err)
	}
	defer destFile.Close()

	// Copy the response body to the destination file with progress tracking
	written, err := d.copyWithProgress(destFile, resp.Body, resp.ContentLength, url)
	if err != nil {
		// Clean up the partial file on error
		os.Remove(destPath)
		return fmt.Errorf("failed to write to destination file: %w", err)
	}

	log.Infof("Successfully downloaded %s (%d bytes) to %s", url, written, destPath)
	return nil
}

// progressReader wraps an io.Reader to provide progress logging
type progressReader struct {
	reader      io.Reader
	totalSize   int64
	written     int64
	url         string
	lastLogTime time.Time
	logInterval time.Duration
}

func newProgressReader(reader io.Reader, totalSize int64, url string) *progressReader {
	return &progressReader{
		reader:      reader,
		totalSize:   totalSize,
		url:         url,
		lastLogTime: time.Now(),
		logInterval: 10 * time.Second,
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.written += int64(n)

		// Log progress periodically
		now := time.Now()
		if now.Sub(pr.lastLogTime) >= pr.logInterval {
			if pr.totalSize > 0 {
				progress := float64(pr.written) / float64(pr.totalSize) * 100
				log.Infof("Download progress for %s: %.1f%% (%d/%d bytes)", pr.url, progress, pr.written, pr.totalSize)
			} else {
				log.Infof("Download progress for %s: %d bytes", pr.url, pr.written)
			}
			pr.lastLogTime = now
		}
	}
	return n, err
}

// copyWithProgress copies data from src to dst with progress logging using io.Copy
func (d *Downloader) copyWithProgress(dst io.Writer, src io.Reader, totalSize int64, url string) (int64, error) {
	progressSrc := newProgressReader(src, totalSize, url)
	return io.Copy(dst, progressSrc)
}

// CalculateChecksum calculates the SHA256 checksum of a file
func (d *Downloader) CalculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// GetFileSize returns the size of a file
func (d *Downloader) GetFileSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}
	return info.Size(), nil
}

// DownloadWithRetry downloads a file with retry logic
func (d *Downloader) DownloadWithRetry(ctx context.Context, url, destPath string, maxRetries int) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Infof("Download attempt %d/%d for %s", attempt, maxRetries, url)

		err := d.DownloadFile(ctx, url, destPath)
		if err == nil {
			return nil
		}

		lastErr = err
		log.Errorf("Download attempt %d failed: %v", attempt, err)

		if attempt < maxRetries {
			// Wait before retrying (exponential backoff)
			waitTime := time.Duration(attempt) * 5 * time.Second
			log.Infof("Waiting %v before retry...", waitTime)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
				// Continue to next attempt
			}
		}
	}

	return fmt.Errorf("download failed after %d attempts: %w", maxRetries, lastErr)
}
