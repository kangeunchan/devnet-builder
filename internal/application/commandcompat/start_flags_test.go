package commandcompat

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestFilterDockerStartArgs_RemovesUnsupportedFlag(t *testing.T) {
	resetStartFlagCaches(t)
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`Usage:
  gaiad start [flags]

Flags:
      --home string
      --minimum-gas-prices string
`), nil
	}

	args := []string{"start", "--home", "/data", "--iavl-disable-fastnode"}
	got := FilterDockerStartArgs(context.Background(), "ghcr.io/cosmos/gaia:v25.3.2", "gaiad", args, nil)
	want := []string{"start", "--home", "/data"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered args: got=%v want=%v", got, want)
	}
}

func TestFilterDockerStartArgs_KeepsSupportedFlags(t *testing.T) {
	resetStartFlagCaches(t)
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`Usage:
  gaiad start [flags]

Flags:
      --home string
      --iavl-disable-fastnode
`), nil
	}

	args := []string{"start", "--home", "/data", "--iavl-disable-fastnode"}
	got := FilterDockerStartArgs(context.Background(), "ghcr.io/cosmos/gaia:v25.3.2", "gaiad", args, nil)
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("unexpected filtered args: got=%v want=%v", got, args)
	}
}

func TestFilterDockerStartArgs_KeepsSupportedUnderscoreFlags(t *testing.T) {
	resetStartFlagCaches(t)
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`Usage:
  gaiad start [flags]

Flags:
      --home string
      --db_backend string
`), nil
	}

	args := []string{"start", "--home", "/data", "--db_backend=pebbledb"}
	got := FilterDockerStartArgs(context.Background(), "ghcr.io/cosmos/gaia:v25.3.2", "gaiad", args, nil)
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("unexpected filtered args: got=%v want=%v", got, args)
	}
}

func TestFilterDockerStartArgs_UsesProbeCache(t *testing.T) {
	resetStartFlagCaches(t)

	probeCount := 0
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		probeCount++
		return []byte(`Usage:
  gaiad start [flags]

Flags:
      --home string
`), nil
	}

	input := []string{"start", "--home", "/data"}
	_ = FilterDockerStartArgs(context.Background(), "ghcr.io/cosmos/gaia:v25.3.2", "gaiad", input, nil)
	_ = FilterDockerStartArgs(context.Background(), "ghcr.io/cosmos/gaia:v25.3.2", "gaiad", input, nil)

	if probeCount != 1 {
		t.Fatalf("expected single probe due to cache, got %d", probeCount)
	}
}

func TestFilterLocalStartArgs_ProbeFailureFallsBack(t *testing.T) {
	resetStartFlagCaches(t)
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("error"), errors.New("probe failed")
	}

	args := []string{"start", "--home", "/data", "--iavl-disable-fastnode"}
	got := FilterLocalStartArgs(context.Background(), "/tmp/gaiad", args, nil)
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("expected original args on probe failure, got=%v want=%v", got, args)
	}
}

func resetStartFlagCaches(t *testing.T) {
	t.Helper()

	oldRun := runCommand
	dockerStartHelpCache = sync.Map{}
	localStartHelpCache = sync.Map{}

	t.Cleanup(func() {
		runCommand = oldRun
		dockerStartHelpCache = sync.Map{}
		localStartHelpCache = sync.Map{}
	})
}
