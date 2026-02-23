// Package snapshot provides snapshot fetching and extraction implementations.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

	// DefaultParallelConnections is the default number of parallel HTTP range
	// requests used for large snapshot downloads when the server supports it.
	DefaultParallelConnections = 4

	// MaxParallelConnections clamps parallel range requests to avoid
	// overwhelming snapshot providers and local resources.
	MaxParallelConnections = 16

	downloadProgressStepName = "Downloading snapshot"
)

// DownloadOptions configures the download behavior.
type DownloadOptions struct {
	URL      string
	DestPath string
	CacheKey string // Format: "plugin-network" (e.g., "stable-mainnet", "ault-testnet")
	HomeDir  string
	NoCache  bool
	// ParallelConnections controls range-request parallelism.
	// 0 means "use env/default".
	ParallelConnections int
	Logger              *output.Logger
	Progress            ports.ProgressReporter // Optional progress reporter
}

var (
	sharedDownloadHTTPClient = &http.Client{Transport: newDownloadTransport()}
	downloadPathLocks        sync.Map
)

type snapshotArchiveFormat struct {
	Decompressor string
	Extension    string
	IsTarArchive bool
}

type parallelAttemptResult struct {
	Used       bool
	Downloaded int64
	SkipReason string
}

// Download downloads a snapshot file with retry logic.
// Returns the SnapshotCache entry on success.
func Download(ctx context.Context, opts DownloadOptions) (*SnapshotCache, error) {
	logger := opts.Logger
	if logger == nil {
		logger = output.DefaultLogger
	}

	if err := validateDownloadOptions(opts); err != nil {
		return nil, err
	}

	cached, err := resolveCachedSnapshot(opts, logger)
	if err != nil {
		return nil, err
	}
	if cached != nil {
		return cached, nil
	}

	archive := DetectSnapshotArchive(opts.URL)
	destPath, err := resolveDownloadPath(opts, archive)
	if err != nil {
		return nil, err
	}

	if err := ensureDownloadDir(destPath); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	if err := downloadWithRetry(ctx, opts, logger, destPath); err != nil {
		return nil, err
	}

	info, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat downloaded file: %w", err)
	}

	cache := NewSnapshotCache(opts.CacheKey, destPath, opts.URL, archive.Decompressor, info.Size())
	if opts.CacheKey != "" {
		if err := cache.Save(opts.HomeDir); err != nil {
			logger.Warn("Failed to save cache metadata: %v", err)
		}
	}

	return cache, nil
}

func validateDownloadOptions(opts DownloadOptions) error {
	if strings.TrimSpace(opts.URL) == "" {
		return errors.New("snapshot URL is required")
	}
	return nil
}

func resolveCachedSnapshot(opts DownloadOptions, logger *output.Logger) (*SnapshotCache, error) {
	if opts.NoCache || opts.CacheKey == "" {
		return nil, nil
	}

	cache, err := GetValidCache(opts.HomeDir, opts.CacheKey)
	if err != nil {
		logger.Debug("Cache check failed: %v", err)
		return nil, nil
	}
	if cache != nil {
		logger.Debug("Using cached snapshot (expires in %s)", cache.TimeUntilExpiry().Round(time.Minute))
		return cache, nil
	}

	return nil, nil
}

func resolveDownloadPath(opts DownloadOptions, archive snapshotArchiveFormat) (string, error) {
	destPath := strings.TrimSpace(opts.DestPath)
	if destPath == "" && opts.CacheKey != "" {
		destPath = SnapshotPath(opts.HomeDir, opts.CacheKey, archive.Extension)
	}
	if destPath == "" {
		return "", fmt.Errorf("destination path is required when cache key is not set")
	}
	return destPath, nil
}

func ensureDownloadDir(destPath string) error {
	dir := filepath.Dir(destPath)
	if dir == "" {
		return fmt.Errorf("invalid destination path %q", destPath)
	}
	return os.MkdirAll(dir, 0o755)
}

func downloadWithRetry(ctx context.Context, opts DownloadOptions, logger *output.Logger, destPath string) error {
	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		if attempt > 1 {
			logger.Warn("Retry attempt %d/%d...", attempt, MaxRetries)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(RetryDelay):
			}
		}

		err := downloadFile(ctx, opts.URL, destPath, logger, opts.Progress, opts.ParallelConnections)
		if err == nil {
			return nil
		}

		lastErr = err
		logger.Warn("Download failed: %v", err)
	}

	return fmt.Errorf("failed to download snapshot after %d attempts: %w", MaxRetries, lastErr)
}

// downloadFile performs the actual HTTP download.
func downloadFile(
	ctx context.Context,
	url string,
	destPath string,
	logger *output.Logger,
	progress ports.ProgressReporter,
	parallelConnectionsOpt int,
) error {
	unlock, waitedForLock := lockDownloadPath(destPath)
	defer unlock()
	if waitedForLock && logger != nil {
		logger.Debug(
			"another download is already in progress for %s; waiting on download lock",
			destPath,
		)
	}

	ctx, cancel := withDefaultDownloadTimeout(ctx)
	defer cancel()

	// Temporary file policy:
	// - ".tmp" stores in-progress bytes for resume support.
	// - per-destination lock prevents concurrent writers from corrupting .tmp state.
	tmpPath := destPath + ".tmp"
	resumeOffset, err := partialFileSize(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to check partial download: %w", err)
	}

	if logger != nil {
		logger.StopSpinner()
	}

	parallelConnections := resolveParallelConnections(parallelConnectionsOpt)
	if resumeOffset > 0 && parallelConnections > 1 && logger != nil {
		logger.Debug(
			"partial snapshot detected (%d bytes); resume mode uses a single connection for consistency",
			resumeOffset,
		)
	}
	if resumeOffset == 0 && parallelConnections > 1 {
		parallelAttempt, parallelErr := tryParallelDownload(
			ctx,
			sharedDownloadHTTPClient,
			url,
			tmpPath,
			parallelConnections,
			logger,
			progress,
		)
		if parallelAttempt.Used {
			if parallelErr == nil {
				return finalizeDownloadFile(tmpPath, destPath)
			}
			if logger != nil {
				logger.Warn(
					"Parallel download failed after %d bytes, falling back to single connection: %v",
					parallelAttempt.Downloaded,
					parallelErr,
				)
			}
			if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("failed to clean temporary file after parallel attempt: %w", removeErr)
			}
		} else if parallelAttempt.SkipReason != "" && logger != nil {
			logger.Debug("Parallel download skipped: %s", parallelAttempt.SkipReason)
		}
	}

	return downloadSingleConnection(ctx, sharedDownloadHTTPClient, url, tmpPath, destPath, resumeOffset, logger, progress)
}

func downloadSingleConnection(
	ctx context.Context,
	client *http.Client,
	url string,
	tmpPath string,
	destPath string,
	resumeOffset int64,
	logger *output.Logger,
	progress ports.ProgressReporter,
) error {
	resp, effectiveResumeOffset, resumeAccepted, err := startSingleDownload(ctx, client, url, tmpPath, resumeOffset, logger)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, total, err := writeSingleDownloadResponse(resp, tmpPath, resumeAccepted, effectiveResumeOffset, logger, progress)
	if err != nil {
		return err
	}
	if err := verifyDownloadedFile(tmpPath, total, effectiveResumeOffset); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("%w (cleanup failed: %v)", err, removeErr)
		}
		return err
	}

	return finalizeDownloadFile(tmpPath, destPath)
}

func startSingleDownload(
	ctx context.Context,
	client *http.Client,
	url string,
	tmpPath string,
	resumeOffset int64,
	logger *output.Logger,
) (*http.Response, int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to create request: %w", err)
	}
	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to start download: %w", err)
	}

	resumeAccepted := resumeOffset > 0 && resp.StatusCode == http.StatusPartialContent
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && resumeOffset > 0 {
		resp.Body.Close()
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, 0, false, fmt.Errorf("failed to reset invalid partial download: %w", removeErr)
		}
		return nil, 0, false, fmt.Errorf("resume range rejected by server (status %d), partial download reset", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK && !resumeAccepted {
		resp.Body.Close()
		return nil, 0, false, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	effectiveResumeOffset := resumeOffset
	if resumeOffset > 0 && !resumeAccepted {
		if logger != nil {
			logger.Debug(
				"Server ignored range resume request (status=%d, accept-ranges=%q), restarting download",
				resp.StatusCode,
				resp.Header.Get("Accept-Ranges"),
			)
		}
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			resp.Body.Close()
			return nil, 0, false, fmt.Errorf("failed to reset partial download: %w", removeErr)
		}
		effectiveResumeOffset = 0
	}

	return resp, effectiveResumeOffset, resumeAccepted, nil
}

func writeSingleDownloadResponse(
	resp *http.Response,
	tmpPath string,
	resumeAccepted bool,
	resumeOffset int64,
	logger *output.Logger,
	progress ports.ProgressReporter,
) (int64, int64, error) {
	out, err := openDownloadOutputFile(tmpPath, resumeAccepted)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open destination file: %w", err)
	}

	total := resp.ContentLength
	if resumeAccepted && total > 0 {
		total += resumeOffset
	}
	downloaded := resumeOffset

	tracker := newProgressTracker(logger, progress, total, 500*time.Millisecond)
	reader := &progressReader{
		reader:     resp.Body,
		downloaded: &downloaded,
		tracker:    tracker,
	}

	_, err = io.Copy(out, reader)
	if err != nil {
		_ = out.Close()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			tracker.Fail(downloaded, fmt.Errorf("download interrupted: %w", err))
			return downloaded, total, fmt.Errorf("download interrupted: %w", err)
		}
		tracker.Fail(downloaded, fmt.Errorf("failed to write file: %w", err))
		return downloaded, total, fmt.Errorf("failed to write file: %w", err)
	}

	if err := out.Close(); err != nil {
		tracker.Fail(downloaded, fmt.Errorf("failed to close file: %w", err))
		return downloaded, total, fmt.Errorf("failed to close file: %w", err)
	}

	tracker.Complete(downloaded)
	return downloaded, total, nil
}

func openDownloadOutputFile(path string, appendMode bool) (*os.File, error) {
	if appendMode {
		return os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	}
	return os.Create(path)
}

func verifyDownloadedFile(path string, expectedTotal int64, minSize int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat downloaded file: %w", err)
	}

	if expectedTotal > 0 {
		if info.Size() != expectedTotal {
			return fmt.Errorf("incomplete download: got %d bytes, expected %d bytes", info.Size(), expectedTotal)
		}
		return nil
	}

	if info.Size() <= minSize {
		return errors.New("download completed without known content length and no data was written")
	}

	return nil
}

func finalizeDownloadFile(tmpPath string, destPath string) error {
	if err := os.Rename(tmpPath, destPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("failed to rename file: %w (cleanup failed: %v)", err, removeErr)
		}
		return fmt.Errorf("failed to rename file: %w", err)
	}
	return nil
}

// progressReader wraps an io.Reader and updates raw byte counters only.
type progressReader struct {
	reader     io.Reader
	downloaded *int64
	tracker    *progressTracker
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	*pr.downloaded += int64(n)
	pr.tracker.MaybeReport(*pr.downloaded)
	return n, err
}

type progressTracker struct {
	logger         *output.Logger
	progress       ports.ProgressReporter
	total          int64
	lastReport     time.Time
	reportInterval time.Duration
	startedAt      time.Time
}

func newProgressTracker(
	logger *output.Logger,
	progress ports.ProgressReporter,
	total int64,
	reportInterval time.Duration,
) *progressTracker {
	now := time.Now()
	return &progressTracker{
		logger:         logger,
		progress:       progress,
		total:          total,
		lastReport:     now,
		reportInterval: reportInterval,
		startedAt:      now,
	}
}

func (t *progressTracker) MaybeReport(current int64) {
	if t == nil || time.Since(t.lastReport) < t.reportInterval {
		return
	}
	t.lastReport = time.Now()
	t.report("running", current, nil)
}

func (t *progressTracker) Complete(current int64) {
	if t == nil {
		return
	}
	t.report("completed", current, nil)
}

func (t *progressTracker) Fail(current int64, cause error) {
	if t == nil {
		return
	}
	t.report("failed", current, cause)
}

func (t *progressTracker) report(status string, current int64, cause error) {
	speed := calcAverageSpeed(current, t.startedAt)
	if t.logger != nil {
		t.logger.Progress(current, t.total, speed)
		if status != "running" {
			t.logger.ProgressComplete()
		}
	}
	reportDownloadStep(t.progress, status, current, t.total, speed, cause)
}

func reportDownloadStep(
	progress ports.ProgressReporter,
	status string,
	current int64,
	total int64,
	speed float64,
	cause error,
) {
	if progress == nil {
		return
	}
	step := ports.StepProgress{
		Name:    downloadProgressStepName,
		Status:  status,
		Current: current,
		Total:   total,
		Unit:    "bytes",
		Speed:   speed,
	}
	if cause != nil {
		step.Error = cause.Error()
	}
	progress.ReportStep(step)
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

func withDefaultDownloadTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), DownloadTimeout)
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, DownloadTimeout)
}

func newDownloadTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func lockDownloadPath(path string) (func(), bool) {
	lock, _ := downloadPathLocks.LoadOrStore(path, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	waited := !mu.TryLock()
	if waited {
		mu.Lock()
	}
	return mu.Unlock, waited
}

type probeResult struct {
	size         int64
	rangeSupport bool
}

func tryParallelDownload(
	ctx context.Context,
	client *http.Client,
	url string,
	tmpPath string,
	connections int,
	logger *output.Logger,
	progress ports.ProgressReporter,
) (parallelAttemptResult, error) {
	result := parallelAttemptResult{}

	if connections < 2 {
		result.SkipReason = "parallel connections is less than 2"
		return result, nil
	}

	probe, err := probeRemoteSnapshot(ctx, client, url)
	if err != nil {
		result.SkipReason = fmt.Sprintf("probe failed: %v", err)
		return result, nil
	}
	if !probe.rangeSupport || probe.size <= 0 {
		result.SkipReason = fmt.Sprintf(
			"server does not support ranged download (accept-ranges=%t size=%d)",
			probe.rangeSupport,
			probe.size,
		)
		return result, nil
	}

	if int64(connections) > probe.size {
		connections = int(probe.size)
		if connections < 2 {
			result.SkipReason = fmt.Sprintf("remote size too small for parallel download: %d", probe.size)
			return result, nil
		}
	}
	result.Used = true

	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return result, fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer file.Close()

	if err := file.Truncate(probe.size); err != nil {
		return result, fmt.Errorf("failed to preallocate temporary file: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var downloadedBytes atomic.Int64
	startedAt := time.Now()

	stopProgress := make(chan struct{})
	progressDone := make(chan struct{})
	go reportParallelProgress(stopProgress, progressDone, logger, progress, &downloadedBytes, probe.size, startedAt)

	chunkSize := (probe.size + int64(connections) - 1) / int64(connections)
	errCh := make(chan error, connections)
	var wg sync.WaitGroup

	for i := 0; i < connections; i++ {
		start := int64(i) * chunkSize
		if start >= probe.size {
			break
		}

		end := start + chunkSize - 1
		if end >= probe.size {
			end = probe.size - 1
		}

		wg.Add(1)
		go func(rangeStart, rangeEnd int64) {
			defer wg.Done()
			if err := downloadRangeChunk(ctx, client, url, tmpPath, rangeStart, rangeEnd, &downloadedBytes); err != nil {
				errCh <- err
				cancel()
			}
		}(start, end)
	}

	wg.Wait()
	close(stopProgress)
	<-progressDone

	close(errCh)
	var firstErr error
	for chunkErr := range errCh {
		if chunkErr != nil {
			firstErr = chunkErr
			break
		}
	}

	result.Downloaded = downloadedBytes.Load()
	speed := calcAverageSpeed(result.Downloaded, startedAt)
	if logger != nil {
		logger.Progress(result.Downloaded, probe.size, speed)
		logger.ProgressComplete()
	}

	if firstErr != nil {
		reportDownloadStep(progress, "failed", result.Downloaded, probe.size, speed, firstErr)
		return result, firstErr
	}

	if result.Downloaded != probe.size {
		err := fmt.Errorf(
			"incomplete parallel download: got %d bytes, expected %d bytes",
			result.Downloaded,
			probe.size,
		)
		reportDownloadStep(progress, "failed", result.Downloaded, probe.size, speed, err)
		return result, err
	}

	reportDownloadStep(progress, "completed", result.Downloaded, probe.size, speed, nil)
	return result, nil
}

func probeRemoteSnapshot(ctx context.Context, client *http.Client, url string) (probeResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return probeResult{}, err
	}

	resp, err := client.Do(req)
	headErr := err
	if err == nil {
		defer resp.Body.Close()

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			size := resp.ContentLength
			rangeSupport := strings.Contains(strings.ToLower(strings.TrimSpace(resp.Header.Get("Accept-Ranges"))), "bytes")
			if rangeSupport && size > 0 {
				return probeResult{size: size, rangeSupport: true}, nil
			}
			if size > 0 {
				return probeResult{size: size, rangeSupport: false}, nil
			}
			headErr = fmt.Errorf("head response missing usable content-length (status=%d)", resp.StatusCode)
		} else {
			headErr = fmt.Errorf("head probe failed with status %d", resp.StatusCode)
		}
	}

	rangeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return probeResult{}, err
	}
	rangeReq.Header.Set("Range", "bytes=0-0")

	rangeResp, err := client.Do(rangeReq)
	if err != nil {
		if headErr != nil {
			return probeResult{}, fmt.Errorf("%v; range probe error: %w", headErr, err)
		}
		return probeResult{}, err
	}
	defer rangeResp.Body.Close()

	switch rangeResp.StatusCode {
	case http.StatusPartialContent:
		size, ok := parseContentRangeTotal(rangeResp.Header.Get("Content-Range"))
		if !ok || size <= 0 {
			return probeResult{}, fmt.Errorf("range probe returned 206 but invalid content-range %q", rangeResp.Header.Get("Content-Range"))
		}
		return probeResult{size: size, rangeSupport: true}, nil
	case http.StatusOK:
		if rangeResp.ContentLength > 0 {
			return probeResult{
				size:         rangeResp.ContentLength,
				rangeSupport: strings.Contains(strings.ToLower(strings.TrimSpace(rangeResp.Header.Get("Accept-Ranges"))), "bytes"),
			}, nil
		}
		return probeResult{}, fmt.Errorf("range probe returned 200 without content-length")
	default:
		return probeResult{}, fmt.Errorf("range probe failed with status %d", rangeResp.StatusCode)
	}
}

func parseContentRangeTotal(contentRange string) (int64, bool) {
	v := strings.TrimSpace(contentRange)
	if v == "" {
		return 0, false
	}

	slash := strings.LastIndex(v, "/")
	if slash < 0 || slash+1 >= len(v) {
		return 0, false
	}

	totalPart := strings.TrimSpace(v[slash+1:])
	if totalPart == "" || totalPart == "*" {
		return 0, false
	}

	total, err := strconv.ParseInt(totalPart, 10, 64)
	if err != nil || total <= 0 {
		return 0, false
	}

	return total, true
}

func downloadRangeChunk(
	ctx context.Context,
	client *http.Client,
	url string,
	tmpPath string,
	start int64,
	end int64,
	downloaded *atomic.Int64,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create range request %d-%d: %w", start, end, err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("range request %d-%d failed: %w", start, end, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("range request %d-%d returned status %d", start, end, resp.StatusCode)
	}

	file, err := os.OpenFile(tmpPath, os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open temporary file for range %d-%d: %w", start, end, err)
	}
	defer file.Close()

	buf := make([]byte, 256*1024)
	offset := start
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := file.WriteAt(buf[:n], offset)
			if writeErr != nil {
				return fmt.Errorf("failed writing range %d-%d: %w", start, end, writeErr)
			}
			if written != n {
				return fmt.Errorf("short write for range %d-%d", start, end)
			}
			offset += int64(n)
			downloaded.Add(int64(n))
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("range read failed for %d-%d: %w", start, end, readErr)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	expectedBytes := end - start + 1
	if got := offset - start; got != expectedBytes {
		return fmt.Errorf("range %d-%d incomplete: got %d bytes, expected %d", start, end, got, expectedBytes)
	}

	return nil
}

func reportParallelProgress(
	stop <-chan struct{},
	done chan<- struct{},
	logger *output.Logger,
	progress ports.ProgressReporter,
	downloaded *atomic.Int64,
	total int64,
	startedAt time.Time,
) {
	defer close(done)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			current := downloaded.Load()
			speed := calcAverageSpeed(current, startedAt)
			if logger != nil {
				logger.Progress(current, total, speed)
			}
			reportDownloadStep(progress, "running", current, total, speed, nil)
		}
	}
}

func calcAverageSpeed(downloaded int64, startedAt time.Time) float64 {
	elapsed := time.Since(startedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(downloaded) / elapsed
}

func resolveParallelConnections(optValue int) int {
	if optValue > 0 {
		if optValue > MaxParallelConnections {
			return MaxParallelConnections
		}
		return optValue
	}

	raw := strings.TrimSpace(os.Getenv("DEVNET_SNAPSHOT_PARALLEL"))
	if raw == "" {
		return DefaultParallelConnections
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultParallelConnections
	}
	if value < 1 {
		return 1
	}
	if value > MaxParallelConnections {
		return MaxParallelConnections
	}
	return value
}

// DetectSnapshotArchive determines archive/decompression metadata from URL.
func DetectSnapshotArchive(url string) snapshotArchiveFormat {
	normalizedURL := strings.ToLower(strings.TrimSpace(url))

	switch {
	case strings.HasSuffix(normalizedURL, ".tar.zst"), strings.HasSuffix(normalizedURL, ".zst"):
		return snapshotArchiveFormat{
			Decompressor: "zstd",
			Extension:    ".tar.zst",
			IsTarArchive: true,
		}
	case strings.HasSuffix(normalizedURL, ".tar.lz4"), strings.HasSuffix(normalizedURL, ".lz4"):
		return snapshotArchiveFormat{
			Decompressor: "lz4",
			Extension:    ".tar.lz4",
			IsTarArchive: true,
		}
	case strings.HasSuffix(normalizedURL, ".tar.gz"), strings.HasSuffix(normalizedURL, ".tgz"):
		return snapshotArchiveFormat{
			Decompressor: "gzip",
			Extension:    ".tar.gz",
			IsTarArchive: true,
		}
	case strings.HasSuffix(normalizedURL, ".tar"):
		return snapshotArchiveFormat{
			Decompressor: "none",
			Extension:    ".tar",
			IsTarArchive: true,
		}
	default:
		return snapshotArchiveFormat{
			Decompressor: "zstd",
			Extension:    ".tar.zst",
			IsTarArchive: true,
		}
	}
}

// DetectDecompressor returns legacy decompressor/extension values.
// Kept for compatibility with existing callers.
func DetectDecompressor(url string) (decompressor string, extension string) {
	archive := DetectSnapshotArchive(url)
	return archive.Decompressor, archive.Extension
}

// GetSnapshotSize returns the size of a remote snapshot without downloading it.
func GetSnapshotSize(ctx context.Context, url string) (int64, error) {
	ctx, cancel := withDefaultDownloadTimeout(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := sharedDownloadHTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to get size: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed with status %d", resp.StatusCode)
	}

	return resp.ContentLength, nil
}
