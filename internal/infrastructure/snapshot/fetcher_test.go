package snapshot

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateExtractorDependencies_MissingLZ4(t *testing.T) {
	oldLookPath := snapshotLookPath
	snapshotLookPath = func(name string) (string, error) {
		if name == "tar" {
			return "/usr/bin/tar", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() {
		snapshotLookPath = oldLookPath
	})

	err := validateExtractorDependencies("lz4")
	if err == nil {
		t.Fatalf("expected dependency error")
	}
	if !strings.Contains(err.Error(), "missing dependency: lz4") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateExtractorDependencies_ZstdWithTarAvailable(t *testing.T) {
	oldLookPath := snapshotLookPath
	snapshotLookPath = func(name string) (string, error) {
		switch name {
		case "zstd", "tar":
			return "/usr/bin/" + name, nil
		default:
			return "", errors.New("not found")
		}
	}
	t.Cleanup(func() {
		snapshotLookPath = oldLookPath
	})

	if err := validateExtractorDependencies("zstd"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
