package nodeconfig

import (
	"fmt"
	"os"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/types"
)

func TestNodeInitializerConfig_UsesExplicitBinaryAndHome(t *testing.T) {
	initializer := NewNodeInitializerWithConfig(NodeInitializerConfig{
		Mode:          types.ExecutionModeDocker,
		DockerImage:   "docker.io/example/network:v1.0.0",
		BinaryPath:    "/tmp/bin/customd",
		BinaryName:    "chaind",
		DockerHomeDir: "/home/chaind",
	})

	if got := initializer.localBinaryPath(); got != "/tmp/bin/customd" {
		t.Fatalf("localBinaryPath() = %q, want %q", got, "/tmp/bin/customd")
	}
	if got := initializer.containerBinary(); got != "chaind" {
		t.Fatalf("containerBinary() = %q, want %q", got, "chaind")
	}
	if got := initializer.containerHomeDir(); got != "/home/chaind" {
		t.Fatalf("containerHomeDir() = %q, want %q", got, "/home/chaind")
	}
}

func TestNodeInitializerConfig_DefaultDockerHomeAndBinaryFallback(t *testing.T) {
	initializer := NewNodeInitializerWithConfig(NodeInitializerConfig{
		Mode:        types.ExecutionModeDocker,
		DockerImage: "docker.io/cosmoshub/gaia:v17.0.0",
	})

	if got := initializer.containerHomeDir(); got != defaultContainerHomeDir {
		t.Fatalf("containerHomeDir() = %q, want %q", got, defaultContainerHomeDir)
	}
	if got := initializer.containerBinary(); got != defaultFallbackBinary {
		t.Fatalf("containerBinary() = %q, want %q", got, defaultFallbackBinary)
	}
}

func TestNodeInitializerWithBinary_DerivesContainerBinaryFromPath(t *testing.T) {
	initializer := NewNodeInitializerWithBinary(
		types.ExecutionModeDocker,
		"docker.io/example/network:v1.0.0",
		"/opt/devnet/bin/networkd",
		nil,
	)

	if got := initializer.containerBinary(); got != "networkd" {
		t.Fatalf("containerBinary() = %q, want %q", got, "networkd")
	}
}

func TestNodeInitializerConfig_UsesBinaryNameForLocalPathWhenNoBinaryPath(t *testing.T) {
	initializer := NewNodeInitializerWithConfig(NodeInitializerConfig{
		Mode:       types.ExecutionModeLocal,
		BinaryName: "gaiad",
	})

	if got := initializer.localBinaryPath(); got != "gaiad" {
		t.Fatalf("localBinaryPath() = %q, want %q", got, "gaiad")
	}
}

func TestNodeInitializerConfig_DockerBaseArgsIncludesUserMapping(t *testing.T) {
	initializer := NewNodeInitializerWithConfig(NodeInitializerConfig{
		Mode:        types.ExecutionModeDocker,
		DockerImage: "docker.io/example/network:v1.0.0",
		BinaryName:  "chaind",
	})

	args := initializer.dockerBaseArgs("/tmp/node0")
	wantUser := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())

	for idx := 0; idx < len(args); idx++ {
		if args[idx] == "--user" {
			if idx+1 >= len(args) {
				t.Fatalf("dockerBaseArgs() missing value after --user")
			}
			if got := args[idx+1]; got != wantUser {
				t.Fatalf("dockerBaseArgs() user mapping = %q, want %q", got, wantUser)
			}
			return
		}
	}

	t.Fatalf("dockerBaseArgs() missing --user argument: %#v", args)
}

func TestWithDockerStdinAttached_InsertsInteractiveFlagBeforeImage(t *testing.T) {
	baseArgs := []string{
		"run", "--rm",
		"--user", "1000:1000",
		"-e", "HOME=/data",
		"-v", "/tmp/node0:/data",
		"--entrypoint", "gaiad",
		"ghcr.io/cosmos/gaia:v25.3.2",
	}

	got := withDockerStdinAttached(baseArgs)

	want := []string{
		"run", "--rm",
		"--user", "1000:1000",
		"-e", "HOME=/data",
		"-v", "/tmp/node0:/data",
		"--entrypoint", "gaiad",
		"-i",
		"ghcr.io/cosmos/gaia:v25.3.2",
	}

	if len(got) != len(want) {
		t.Fatalf("withDockerStdinAttached() length = %d, want %d, got %#v", len(got), len(want), got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("withDockerStdinAttached()[%d] = %q, want %q (full: %#v)", idx, got[idx], want[idx], got)
		}
	}
}

func TestWithDockerStdinAttached_EmptyArgsNoop(t *testing.T) {
	var args []string
	got := withDockerStdinAttached(args)
	if len(got) != 0 {
		t.Fatalf("withDockerStdinAttached(empty) = %#v, want empty", got)
	}
}
