package upgrade

import (
	"context"
	"errors"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/types"
)

func TestSwitchBinaryUseCase_Execute_LocalMode(t *testing.T) {
	t.Parallel()

	pid := 999
	nodes := []*ports.NodeMetadata{{Index: 0, Name: "node0", HomeDir: "/tmp/node0", PID: &pid}}

	setActiveRef := ""
	uc := NewSwitchBinaryUseCase(
		&mockDevnetRepo{},
		&mockNodeRepo{loadAllFunc: func(ctx context.Context, homeDir string) ([]*ports.NodeMetadata, error) { return nodes, nil }},
		&mockProcessExecutor{},
		&mockBinaryCache{setActiveFunc: func(ref string) error { setActiveRef = ref; return nil }},
		&testLogger{},
	)

	out, err := uc.Execute(context.Background(), dto.SwitchBinaryInput{
		HomeDir:      "/tmp/devnet",
		Mode:         types.ExecutionModeLocal,
		TargetBinary: "/tmp/new-stabled",
		CacheRef:     "abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.NewBinary != "/tmp/new-stabled" {
		t.Fatalf("unexpected new binary: %s", out.NewBinary)
	}
	if setActiveRef != "abc123" {
		t.Fatalf("SetActive called with %q", setActiveRef)
	}
	if out.NodesRestarted != 1 {
		t.Fatalf("expected 1 restarted node, got %d", out.NodesRestarted)
	}
}

func TestSwitchBinaryUseCase_Execute_Errors(t *testing.T) {
	t.Parallel()

	t.Run("no target", func(t *testing.T) {
		t.Parallel()
		uc := NewSwitchBinaryUseCase(&mockDevnetRepo{}, &mockNodeRepo{}, &mockProcessExecutor{}, &mockBinaryCache{}, &testLogger{})
		_, err := uc.Execute(context.Background(), dto.SwitchBinaryInput{HomeDir: "/tmp/devnet", Mode: types.ExecutionModeLocal})
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("set active fails", func(t *testing.T) {
		t.Parallel()
		uc := NewSwitchBinaryUseCase(
			&mockDevnetRepo{},
			&mockNodeRepo{},
			&mockProcessExecutor{},
			&mockBinaryCache{setActiveFunc: func(ref string) error { return errors.New("cache fail") }},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.SwitchBinaryInput{HomeDir: "/tmp/devnet", Mode: types.ExecutionModeLocal, TargetBinary: "/tmp/bin", CacheRef: "ref"})
		if err == nil || err.Error() != "failed to activate binary: cache fail" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("node load fails", func(t *testing.T) {
		t.Parallel()
		uc := NewSwitchBinaryUseCase(
			&mockDevnetRepo{},
			&mockNodeRepo{loadAllFunc: func(ctx context.Context, homeDir string) ([]*ports.NodeMetadata, error) {
				return nil, errors.New("nodes fail")
			}},
			&mockProcessExecutor{},
			&mockBinaryCache{},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.SwitchBinaryInput{HomeDir: "/tmp/devnet", Mode: types.ExecutionModeDocker, TargetImage: "img"})
		if err == nil || err.Error() != "failed to load nodes: nodes fail" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestSwitchBinaryUseCase_BuildStartCommand(t *testing.T) {
	t.Parallel()

	uc := &SwitchBinaryUseCase{}
	cmd := uc.buildStartCommand(&ports.NodeMetadata{HomeDir: "/tmp/node0"}, &ports.DevnetMetadata{}, "", 100)
	if cmd.Binary != "stabled" {
		t.Fatalf("expected default binary, got %s", cmd.Binary)
	}
	if len(cmd.Args) < 3 || cmd.Args[0] != "start" {
		t.Fatalf("unexpected args: %+v", cmd.Args)
	}
}

func TestPidHandle_Methods(t *testing.T) {
	t.Parallel()

	h := &pidHandle{pid: 1234}
	if got := h.PID(); got != 1234 {
		t.Fatalf("PID = %d", got)
	}
	if h.IsRunning() {
		t.Fatalf("expected IsRunning false")
	}
	if err := h.Wait(); err != nil {
		t.Fatalf("Wait error: %v", err)
	}
	if err := h.Kill(); err != nil {
		t.Fatalf("Kill error: %v", err)
	}
}
