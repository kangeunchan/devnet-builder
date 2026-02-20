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

	if err := downloadFile(context.Background(), server.URL, destPath, logger, nil); err != nil {
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

	if err := downloadFile(context.Background(), server.URL, destPath, logger, nil); err != nil {
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
