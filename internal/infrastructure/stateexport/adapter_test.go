package stateexport

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRewriteHomeDirArgsForDocker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		hostHome      string
		containerHome string
		want          []string
	}{
		{
			name:          "rewrites --home value",
			args:          []string{"export", "--home", "/tmp/state-export"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home", "/data"},
		},
		{
			name:          "rewrites --home equals syntax",
			args:          []string{"export", "--home=/tmp/state-export"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home=/data"},
		},
		{
			name:          "rewrites nested home path",
			args:          []string{"export", "--home", "/tmp/state-export/node0"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home", "/data/node0"},
		},
		{
			name:          "leaves unrelated absolute path untouched",
			args:          []string{"export", "--home", "/opt/other-home"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home", "/opt/other-home"},
		},
		{
			name:          "leaves relative home untouched",
			args:          []string{"export", "--home", "relative-home"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home", "relative-home"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := append([]string(nil), tc.args...)
			got := rewriteHomeDirArgsForDocker(tc.args, tc.hostHome, tc.containerHome)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rewriteHomeDirArgsForDocker() = %#v, want %#v", got, tc.want)
			}

			if !reflect.DeepEqual(tc.args, original) {
				t.Fatalf("rewriteHomeDirArgsForDocker mutated input args")
			}
		})
	}
}

func TestProbeDockerExportCommandHelp_UsesCache(t *testing.T) {
	resetDockerExportProbeState(t)

	callCount := 0
	stateExportRunCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		callCount++
		return []byte("ok"), nil
	}

	if err := probeDockerExportCommandHelp(context.Background(), "ghcr.io/cosmos/gaia:v25.3.2", "gaiad"); err != nil {
		t.Fatalf("unexpected probe error: %v", err)
	}
	if err := probeDockerExportCommandHelp(context.Background(), "ghcr.io/cosmos/gaia:v25.3.2", "gaiad"); err != nil {
		t.Fatalf("unexpected probe error on cached call: %v", err)
	}

	if callCount != 1 {
		t.Fatalf("expected one probe command due to cache, got %d", callCount)
	}
}

func TestProbeDockerExportCommandHelp_ErrorIncludesProbeContext(t *testing.T) {
	resetDockerExportProbeState(t)

	stateExportRunCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("permission denied"), errors.New("exit status 1")
	}

	err := probeDockerExportCommandHelp(context.Background(), "ghcr.io/cosmos/gaia:v25.3.2", "gaiad")
	if err == nil {
		t.Fatalf("expected probe error")
	}
	message := err.Error()
	if !strings.Contains(message, "image=ghcr.io/cosmos/gaia:v25.3.2") {
		t.Fatalf("missing image context in error: %v", err)
	}
	if !strings.Contains(message, "entrypoint=gaiad") {
		t.Fatalf("missing entrypoint context in error: %v", err)
	}
	if !strings.Contains(message, "command=export --help") {
		t.Fatalf("missing command context in error: %v", err)
	}
}

func resetDockerExportProbeState(t *testing.T) {
	t.Helper()

	oldRunCommand := stateExportRunCommand
	dockerExportProbeCache = sync.Map{}

	t.Cleanup(func() {
		stateExportRunCommand = oldRunCommand
		dockerExportProbeCache = sync.Map{}
	})
}
