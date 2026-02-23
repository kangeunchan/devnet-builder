package manage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBinaryPath_ValidExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "test-binary")

	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	result, err := validateBinaryPath(binaryPath)
	if err != nil {
		t.Errorf("validateBinaryPath() failed with valid executable: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("validateBinaryPath() should return absolute path, got: %s", result)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	os.Chdir(tmpDir)

	relPath := "./test-binary"
	result, err = validateBinaryPath(relPath)
	if err != nil {
		t.Errorf("validateBinaryPath() failed with relative path: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("validateBinaryPath() should convert relative path to absolute, got: %s", result)
	}
}

func TestValidateBinaryPath_NotFound(t *testing.T) {
	nonExistentPath := "/nonexistent/path/to/binary"

	_, err := validateBinaryPath(nonExistentPath)
	if err == nil {
		t.Error("validateBinaryPath() should fail with non-existent path")
	}
	if err != nil && !os.IsNotExist(err) {
		expectedMsg := "binary not found at path"
		if len(err.Error()) >= len(expectedMsg) && err.Error()[:len(expectedMsg)] != expectedMsg {
			t.Errorf("Expected error message to start with '%s', got: %v", expectedMsg, err)
		}
	}
}

func TestValidateBinaryPath_NotExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "test-binary")

	if err := os.WriteFile(binaryPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := validateBinaryPath(binaryPath)
	if err == nil {
		t.Error("validateBinaryPath() should fail with non-executable file")
	}
	if err != nil {
		expectedMsg := "binary is not executable"
		if len(err.Error()) >= len(expectedMsg) && err.Error()[:len(expectedMsg)] != expectedMsg {
			t.Errorf("Expected error message to start with '%s', got: %v", expectedMsg, err)
		}
	}
}

func TestValidateBinaryPath_IsDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := validateBinaryPath(tmpDir)
	if err == nil {
		t.Error("validateBinaryPath() should fail when path is a directory")
	}
	if err != nil {
		expectedMsg := "path is a directory"
		if len(err.Error()) >= len(expectedMsg) && err.Error()[:len(expectedMsg)] != expectedMsg {
			t.Errorf("Expected error message to start with '%s', got: %v", expectedMsg, err)
		}
	}
}

func TestValidateBinaryPath_WithSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	realBinary := filepath.Join(tmpDir, "real-binary")
	symlinkPath := filepath.Join(tmpDir, "symlink-binary")

	if err := os.WriteFile(realBinary, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	if err := os.Symlink(realBinary, symlinkPath); err != nil {
		t.Skipf("Symlink creation not supported on this system: %v", err)
	}

	result, err := validateBinaryPath(symlinkPath)
	if err != nil {
		t.Errorf("validateBinaryPath() should work with symlinks: %v", err)
	}

	if !filepath.IsAbs(result) {
		t.Errorf("validateBinaryPath() should return absolute path for symlink, got: %s", result)
	}
}

func TestValidateBinaryPath_PathWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	dirWithSpaces := filepath.Join(tmpDir, "dir with spaces")
	if err := os.Mkdir(dirWithSpaces, 0755); err != nil {
		t.Fatalf("Failed to create directory with spaces: %v", err)
	}

	binaryPath := filepath.Join(dirWithSpaces, "test binary")

	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	result, err := validateBinaryPath(binaryPath)
	if err != nil {
		t.Errorf("validateBinaryPath() should handle paths with spaces: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("validateBinaryPath() should return absolute path, got: %s", result)
	}
}

func TestRunDeployPreflight_DockerPermissionDenied(t *testing.T) {
	restoreDeployPreflightStubs(t)
	deployRunCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "info" {
			return []byte("permission denied while trying to connect to the Docker daemon socket"), errors.New("exit status 1")
		}
		return []byte("unexpected command"), nil
	}

	err := runDeployPreflight(context.Background(), deployPreflightOptions{
		Mode:        "docker",
		DockerImage: "ghcr.io/cosmos/gaia:v25.3.2",
		BinaryName:  "gaiad",
	})
	if err == nil {
		t.Fatalf("expected preflight error, got nil")
	}
	if !strings.Contains(err.Error(), "docker daemon is not accessible") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeployPreflight_MissingDecompressor(t *testing.T) {
	restoreDeployPreflightStubs(t)
	deployLookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}

	err := runDeployPreflight(context.Background(), deployPreflightOptions{
		Mode: "local",
		Fork: true,
	})
	if err == nil {
		t.Fatalf("expected preflight error, got nil")
	}
	if !strings.Contains(err.Error(), "snapshot decompressor not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeployPreflight_ImageCommandIncompatibility(t *testing.T) {
	restoreDeployPreflightStubs(t)
	deployRunCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "docker" {
			return nil, errors.New("unexpected command")
		}
		if len(args) > 0 && args[0] == "info" {
			return []byte("ok"), nil
		}
		if strings.Contains(strings.Join(args, " "), "start --help") {
			return []byte("Error: unknown flag: --iavl-disable-fastnode"), errors.New("exit status 1")
		}
		return []byte("ok"), nil
	}

	err := runDeployPreflight(context.Background(), deployPreflightOptions{
		Mode:        "docker",
		DockerImage: "ghcr.io/cosmos/gaia:v25.3.2",
		BinaryName:  "gaiad",
	})
	if err == nil {
		t.Fatalf("expected preflight error, got nil")
	}
	if !strings.Contains(err.Error(), "image command probe failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "command=start --help") {
		t.Fatalf("missing command context in error: %v", err)
	}
}

func TestRunDeployPreflight_DockerSuccess(t *testing.T) {
	restoreDeployPreflightStubs(t)

	var startProbeCalled bool
	var exportProbeCalled bool

	deployLookPath = func(file string) (string, error) {
		if file == "zstd" {
			return "/usr/bin/zstd", nil
		}
		return "", errors.New("not found")
	}
	deployRunCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "docker" {
			return nil, errors.New("unexpected command")
		}
		if len(args) > 0 && args[0] == "info" {
			return []byte("ok"), nil
		}

		joined := strings.Join(args, " ")
		if strings.Contains(joined, "start --help") {
			startProbeCalled = true
		}
		if strings.Contains(joined, "export --help") {
			exportProbeCalled = true
		}
		return []byte("ok"), nil
	}

	err := runDeployPreflight(context.Background(), deployPreflightOptions{
		Mode:        "docker",
		Fork:        true,
		DockerImage: "ghcr.io/cosmos/gaia:v25.3.2",
		BinaryName:  "gaiad",
	})
	if err != nil {
		t.Fatalf("expected preflight success, got error: %v", err)
	}
	if !startProbeCalled {
		t.Fatalf("expected start --help probe to be executed")
	}
	if !exportProbeCalled {
		t.Fatalf("expected export --help probe to be executed")
	}
}

func restoreDeployPreflightStubs(t *testing.T) {
	t.Helper()

	oldRun := deployRunCommand
	oldLookPath := deployLookPath
	t.Cleanup(func() {
		deployRunCommand = oldRun
		deployLookPath = oldLookPath
	})
}
