package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerBuilderCleanupOnError_LIFOAndMultierr(t *testing.T) {
	builder := NewServerBuilder(&Config{DataDir: t.TempDir()})
	order := make([]string, 0, 2)

	builder.pushCleanup(func() error {
		order = append(order, "first")
		return errors.New("cleanup-first")
	})
	builder.pushCleanup(func() error {
		order = append(order, "second")
		return errors.New("cleanup-second")
	})
	builder.err = errors.New("build-failed")

	_, err := builder.Build()
	if err == nil {
		t.Fatal("expected build error")
	}

	if len(order) != 2 || order[0] != "second" || order[1] != "first" {
		t.Fatalf("cleanup order mismatch: %+v", order)
	}

	msg := err.Error()
	if !strings.Contains(msg, "build-failed") {
		t.Fatalf("expected build error in aggregated message: %v", err)
	}
	if !strings.Contains(msg, "cleanup-first") || !strings.Contains(msg, "cleanup-second") {
		t.Fatalf("expected cleanup errors in aggregated message: %v", err)
	}
}

func TestServerBuilderBuildSuccess_DoesNotRunCleanup(t *testing.T) {
	builder := NewServerBuilder(&Config{DataDir: t.TempDir()})
	cleanupCalled := false
	builder.pushCleanup(func() error {
		cleanupCalled = true
		return nil
	})

	server, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if server == nil {
		t.Fatal("expected server instance")
	}
	if cleanupCalled {
		t.Fatal("cleanup must not run on successful build")
	}
	if len(builder.cleanupStack) != 0 {
		t.Fatalf("cleanup stack should be cleared, got %d", len(builder.cleanupStack))
	}
}

func TestServerBuilderWithDataDir(t *testing.T) {
	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "daemon-data")

	builder := NewServerBuilder(&Config{DataDir: dataDir}).WithDataDir()
	if builder.err != nil {
		t.Fatalf("WithDataDir failed: %v", builder.err)
	}

	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("expected data dir to exist: %v", err)
	}
}
