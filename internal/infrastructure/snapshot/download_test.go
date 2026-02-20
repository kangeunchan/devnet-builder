package snapshot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/output"
)

func TestDownloadFile_ResumeFromPartialFile(t *testing.T) {
	fullData := []byte(strings.Repeat("resume-download-data-", 4096))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(fullData)))
			_, _ = w.Write(fullData)
			return
		}

		var start int
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start); err != nil || start < 0 || start >= len(fullData) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		chunk := fullData[start:]
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(fullData)-1, len(fullData)))
		w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(chunk)
	}))
	defer server.Close()

	destPath := filepath.Join(t.TempDir(), "snapshot.tar.lz4")
	partialPath := destPath + ".tmp"
	partial := fullData[:len(fullData)/3]
	if err := os.WriteFile(partialPath, partial, 0o644); err != nil {
		t.Fatalf("write partial file: %v", err)
	}

	logger := output.NewLogger()
	logger.SetNoColor(true)

	if err := downloadFile(context.Background(), server.URL, destPath, logger, nil, 0); err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read final file: %v", err)
	}
	if string(got) != string(fullData) {
		t.Fatalf("downloaded data mismatch: got=%d want=%d", len(got), len(fullData))
	}
}

func TestDownloadFile_RestartWhenRangeNotSupported(t *testing.T) {
	fullData := []byte(strings.Repeat("full-download-data-", 4096))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a server that ignores Range and always returns 200/full body.
		w.Header().Set("Content-Length", strconv.Itoa(len(fullData)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fullData)
	}))
	defer server.Close()

	destPath := filepath.Join(t.TempDir(), "snapshot.tar.lz4")
	partialPath := destPath + ".tmp"
	partial := fullData[:len(fullData)/4]
	if err := os.WriteFile(partialPath, partial, 0o644); err != nil {
		t.Fatalf("write partial file: %v", err)
	}

	logger := output.NewLogger()
	logger.SetNoColor(true)

	if err := downloadFile(context.Background(), server.URL, destPath, logger, nil, 0); err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read final file: %v", err)
	}
	if string(got) != string(fullData) {
		t.Fatalf("downloaded data mismatch: got=%d want=%d", len(got), len(fullData))
	}
}

func TestDownloadFile_ParallelRangeDownload(t *testing.T) {
	t.Setenv("DEVNET_SNAPSHOT_PARALLEL", "4")

	fullData := []byte(strings.Repeat("parallel-download-data-", 1024*4)) // ~92 MiB
	var rangeReqCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(fullData)))
			w.WriteHeader(http.StatusOK)
			return
		case http.MethodGet:
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "" {
				w.Header().Set("Content-Length", strconv.Itoa(len(fullData)))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(fullData)
				return
			}

			var start, end int
			if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			if start < 0 || end >= len(fullData) || start > end {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}

			rangeReqCount.Add(1)
			chunk := fullData[start : end+1]
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(fullData)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(chunk)
			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	destPath := filepath.Join(t.TempDir(), "snapshot.tar.lz4")

	logger := output.NewLogger()
	logger.SetNoColor(true)

	if err := downloadFile(context.Background(), server.URL, destPath, logger, nil, 0); err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read final file: %v", err)
	}
	if string(got) != string(fullData) {
		t.Fatalf("downloaded data mismatch: got=%d want=%d", len(got), len(fullData))
	}
	if rangeReqCount.Load() < 2 {
		t.Fatalf("expected multiple range requests, got %d", rangeReqCount.Load())
	}
}

func TestProbeRemoteSnapshot_FallbackToRange(t *testing.T) {
	fullData := []byte(strings.Repeat("probe-fallback-", 1024))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			if r.Header.Get("Range") == "bytes=0-0" {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(fullData)))
				w.Header().Set("Content-Length", "1")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(fullData[:1])
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	probe, err := probeRemoteSnapshot(context.Background(), sharedDownloadHTTPClient, server.URL)
	if err != nil {
		t.Fatalf("probeRemoteSnapshot failed: %v", err)
	}
	if !probe.rangeSupport {
		t.Fatalf("expected rangeSupport=true")
	}
	if probe.size != int64(len(fullData)) {
		t.Fatalf("unexpected size: got=%d want=%d", probe.size, len(fullData))
	}
}

func TestResolveParallelConnections_OptionPrecedence(t *testing.T) {
	t.Setenv("DEVNET_SNAPSHOT_PARALLEL", "9")

	if got := resolveParallelConnections(3); got != 3 {
		t.Fatalf("expected option value to win, got %d", got)
	}
	if got := resolveParallelConnections(0); got != 9 {
		t.Fatalf("expected env value, got %d", got)
	}
}

func TestResolveDownloadPath_RequiresDestinationWithoutCacheKey(t *testing.T) {
	archive := DetectSnapshotArchive("https://example.com/snapshot.tar.lz4")
	_, err := resolveDownloadPath(DownloadOptions{
		DestPath: "",
		CacheKey: "",
		HomeDir:  t.TempDir(),
	}, archive)
	if err == nil {
		t.Fatalf("expected resolveDownloadPath to fail when both dest path and cache key are empty")
	}
}

func TestDetectSnapshotArchive(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		decompressor string
		extension    string
	}{
		{name: "zstd", url: "https://example.com/file.tar.zst", decompressor: "zstd", extension: ".tar.zst"},
		{name: "lz4", url: "https://example.com/file.tar.lz4", decompressor: "lz4", extension: ".tar.lz4"},
		{name: "gzip", url: "https://example.com/file.tgz", decompressor: "gzip", extension: ".tar.gz"},
		{name: "tar", url: "https://example.com/file.tar", decompressor: "none", extension: ".tar"},
		{name: "default", url: "https://example.com/file.unknown", decompressor: "zstd", extension: ".tar.zst"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := DetectSnapshotArchive(tc.url)
			if got.Decompressor != tc.decompressor {
				t.Fatalf("unexpected decompressor: got=%q want=%q", got.Decompressor, tc.decompressor)
			}
			if got.Extension != tc.extension {
				t.Fatalf("unexpected extension: got=%q want=%q", got.Extension, tc.extension)
			}
			if !got.IsTarArchive {
				t.Fatalf("expected IsTarArchive=true")
			}
		})
	}
}
