package devnet

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type captureLogger struct {
	mu      sync.Mutex
	prints  []string
	println []string
}

func (l *captureLogger) Info(format string, args ...interface{})    {}
func (l *captureLogger) Warn(format string, args ...interface{})    {}
func (l *captureLogger) Error(format string, args ...interface{})   {}
func (l *captureLogger) Debug(format string, args ...interface{})   {}
func (l *captureLogger) Success(format string, args ...interface{}) {}
func (l *captureLogger) SetVerbose(verbose bool)                    {}
func (l *captureLogger) IsVerbose() bool                            { return false }
func (l *captureLogger) Writer() io.Writer                          { return io.Discard }
func (l *captureLogger) ErrWriter() io.Writer                       { return io.Discard }

func (l *captureLogger) Print(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prints = append(l.prints, fmt.Sprintf(format, args...))
}

func (l *captureLogger) Println(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.println = append(l.println, fmt.Sprintf(format, args...))
}

func (l *captureLogger) hasPrintedContaining(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.prints {
		if strings.Contains(line, substr) {
			return true
		}
	}
	for _, line := range l.println {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

func TestStreamFileProgress_TracksTmpAndFinalOutputPaths(t *testing.T) {
	tmpDir := t.TempDir()
	finalPath := filepath.Join(tmpDir, "genesis_output.json")
	tmpPath := finalPath + ".tmp"

	logger := &captureLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := streamFileProgress(
		ctx,
		logger,
		"Patching genesis",
		1024*1024,
		20*time.Millisecond,
		tmpPath,
		finalPath,
	)

	blob1 := make([]byte, 128*1024)
	if err := os.WriteFile(tmpPath, blob1, 0o644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	blob2 := make([]byte, 512*1024)
	if err := os.WriteFile(tmpPath, blob2, 0o644); err != nil {
		t.Fatalf("failed to grow tmp file: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	if err := os.Rename(tmpPath, finalPath); err != nil {
		t.Fatalf("failed to rename tmp to final path: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	stop()

	if !logger.hasPrintedContaining("Patching genesis") {
		t.Fatalf("expected progress output containing label, got prints=%v println=%v", logger.prints, logger.println)
	}
}

func TestFirstExistingFileSize_PicksFirstExistingPath(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "f.bin")

	content := []byte("abcde")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	size, ok := firstExistingFileSize([]string{"", filepath.Join(tmpDir, "missing"), path})
	if !ok {
		t.Fatal("expected existing file size lookup to succeed")
	}
	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}
}
