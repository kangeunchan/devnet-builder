// Package snapshot provides snapshot fetching and extraction implementations.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/internal/output"
)

const (
	// MaxRetries is the maximum number of download retry attempts.
	MaxRetries = 3

	// RetryDelay is the delay between retry attempts.
	RetryDelay = 5 * time.Second

	// DownloadTimeout is the maximum time allowed for a download.
	DownloadTimeout = 30 * time.Minute
)

// DownloadOptions configures the download behavior.
type DownloadOptions struct {
	URL      string
	DestPath string
	CacheKey string // Format: "plugin-network" (e.g., "stable-mainnet", "ault-testnet")
	HomeDir  string
	NoCache  bool
	Logger   *output.Logger
	Progress ports.ProgressReporter // Optional progress reporter
}

// Download downloads a snapshot file with retry logic.
// Returns the SnapshotCache entry on success.
func Download(ctx context.Context, opts DownloadOptions) (*SnapshotCache, error) {
	logger := opts.Logger
	if logger == nil {
		logger = output.DefaultLogger
	}

	// Check cache first (unless NoCache is set)
	if !opts.NoCache && opts.CacheKey != "" {
		cache, err := GetValidCache(opts.HomeDir, opts.CacheKey)
		if err != nil {
			logger.Debug("Cache check failed: %v", err)
		}
		if cache != nil {
			logger.Debug("Using cached snapshot (expires in %s)", cache.TimeUntilExpiry().Round(time.Minute))
			return cache, nil
		}
	}

	// Determine decompressor and file extension
	decompressor, extension := DetectDecompressor(opts.URL)

	// Set destination path if not provided
	destPath := opts.DestPath
	if destPath == "" && opts.CacheKey != "" {
		destPath = SnapshotPath(opts.HomeDir, opts.CacheKey, extension)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	// Download with retries
	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		if attempt > 1 {
			logger.Warn("Retry attempt %d/%d...", attempt, MaxRetries)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(RetryDelay):
			}
		}

		err := downloadFile(ctx, opts.URL, destPath, logger, opts.Progress)
		if err == nil {
			// Get file size
			info, err := os.Stat(destPath)
			if err != nil {
				return nil, fmt.Errorf("failed to stat downloaded file: %w", err)
			}

			// Create cache entry
			cache := NewSnapshotCache(opts.CacheKey, destPath, opts.URL, decompressor, info.Size())

			// Save cache metadata (only if cache key is provided)
			if opts.CacheKey != "" {
				if err := cache.Save(opts.HomeDir); err != nil {
					logger.Warn("Failed to save cache metadata: %v", err)
				}
			}

			return cache, nil
		}

		lastErr = err
		logger.Warn("Download failed: %v", err)
	}

	return nil, fmt.Errorf("failed to download snapshot after %d attempts: %w", MaxRetries, lastErr)
}

// downloadFile performs the actual HTTP download.
func downloadFile(ctx context.Context, url, destPath string, logger *output.Logger, progress ports.ProgressReporter) error {
	// Respect caller context deadline when provided (allows configurable timeout),
	// otherwise fallback to default download timeout.
	timeout := DownloadTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.DeadlineExceeded
		}
		timeout = remaining
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	tmpPath := destPath + ".tmp"
	resumeOffset, err := partialFileSize(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to check partial download: %w", err)
	}

	if logger != nil {
		logger.StopSpinner()
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}

	// Start download
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to start download: %w", err)
	}
	defer resp.Body.Close()

	resumeAccepted := resumeOffset > 0 && resp.StatusCode == http.StatusPartialContent
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && resumeOffset > 0 {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("failed to reset invalid partial download: %w", removeErr)
		}
		return fmt.Errorf("resume range rejected by server (status %d), partial download reset", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK && !resumeAccepted {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// If server does not support range requests, restart from scratch.
	if resumeOffset > 0 && !resumeAccepted {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("failed to reset partial download: %w", removeErr)
		}
		resumeOffset = 0
	}

	// Create destination file
	var out *os.File
	if resumeAccepted {
		out, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND, 0o644)
	} else {
		out, err = os.Create(tmpPath)
	}
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Copy with progress
	total := resp.ContentLength
	if resumeAccepted && total > 0 {
		total += resumeOffset
	}
	downloaded := resumeOffset
	now := time.Now()

	// Create progress reporter
	progressReader := &progressReader{
		reader:         resp.Body,
		total:          total,
		downloaded:     &downloaded,
		logger:         logger,
		progress:       progress,
		lastReport:     now,
		reportInterval: 500 * time.Millisecond, // Update every 500ms for smooth progress
		startTime:      now,
		lastDownloaded: 0,
		lastSpeedCheck: now,
		currentSpeed:   0,
	}

	_, err = io.Copy(out, progressReader)

	// Note: Terminal progress bar removed - progress reported via ProgressReporter

	if err != nil {
		// Keep partial .tmp file for resume on retry.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("download interrupted: %w", err)
		}
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Close file before rename
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if logger != nil {
		logger.Progress(downloaded, total, progressReader.currentSpeed)
		logger.ProgressComplete()
	}

	// Validate downloaded file size matches Content-Length
	// This prevents truncated downloads from being cached as valid
	if total > 0 {
		info, err := os.Stat(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("failed to stat downloaded file: %w", err)
		}
		if info.Size() != total {
			os.Remove(tmpPath)
			return fmt.Errorf("incomplete download: got %d bytes, expected %d bytes", info.Size(), total)
		}
	}

	// Rename temp file to final destination
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// progressReader wraps an io.Reader to report download progress.
type progressReader struct {
	reader         io.Reader
	total          int64
	downloaded     *int64
	logger         *output.Logger
	progress       ports.ProgressReporter // Optional progress reporter
	lastReport     time.Time
	reportInterval time.Duration
	startTime      time.Time
	lastDownloaded int64
	lastSpeedCheck time.Time
	currentSpeed   float64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	*pr.downloaded += int64(n)

	// Report progress periodically
	if time.Since(pr.lastReport) >= pr.reportInterval {
		now := time.Now()

		// Calculate speed (bytes per second)
		timeSinceLastCheck := now.Sub(pr.lastSpeedCheck).Seconds()
		if timeSinceLastCheck > 0 {
			bytesDownloaded := *pr.downloaded - pr.lastDownloaded
			pr.currentSpeed = float64(bytesDownloaded) / timeSinceLastCheck
			pr.lastDownloaded = *pr.downloaded
			pr.lastSpeedCheck = now
		}

		pr.lastReport = now

		// Report step progress if progress reporter is provided
		// Note: Terminal progress bar removed - CLI displays progress via daemon API
		if pr.progress != nil {
			pr.progress.ReportStep(ports.StepProgress{
				Name:    "Downloading snapshot",
				Status:  "running",
				Current: *pr.downloaded,
				Total:   pr.total,
				Unit:    "bytes",
				Speed:   pr.currentSpeed,
			})
		}
		if pr.logger != nil {
			pr.logger.Progress(*pr.downloaded, pr.total, pr.currentSpeed)
		}
	}

	return n, err
}

func partialFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

// DetectDecompressor determines the decompressor and extension from URL.
func DetectDecompressor(url string) (decompressor string, extension string) {
	if strings.HasSuffix(url, ".tar.zst") || strings.HasSuffix(url, ".zst") {
		return "zstd", ".tar.zst"
	}
	if strings.HasSuffix(url, ".tar.lz4") || strings.HasSuffix(url, ".lz4") {
		return "lz4", ".tar.lz4"
	}
	if strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz") {
		return "gzip", ".tar.gz"
	}
	if strings.HasSuffix(url, ".tar") {
		return "none", ".tar"
	}
	return "zstd", ".tar.zst" // Default to zstd
}

// GetSnapshotSize returns the size of a remote snapshot without downloading it.
func GetSnapshotSize(ctx context.Context, url string) (int64, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to get size: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed with status %d", resp.StatusCode)
	}

	return resp.ContentLength, nil
}
