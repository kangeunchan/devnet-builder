package upgrade

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/internal/output"
	"github.com/altuslabsxyz/devnet-builder/types"
)

func newSilentOutputLogger() *output.Logger {
	l := output.NewLogger()
	l.SetJSONMode(true)
	return l
}

func TestResumeUseCase_BasicHelpers(t *testing.T) {
	t.Parallel()

	state := ports.NewUpgradeState("v2", "local", false)
	mgr := &mockStateManager{
		loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil },
	}
	uc := NewResumeUseCase(mgr, &mockStateDetector{}, NewStateTransitioner(), nil, newSilentOutputLogger())

	got, err := uc.CheckState(context.Background())
	if err != nil {
		t.Fatalf("CheckState error: %v", err)
	}
	if got != state {
		t.Fatalf("CheckState state mismatch")
	}

	got, err = uc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if got != state {
		t.Fatalf("GetStatus state mismatch")
	}

	if err := uc.ClearState(context.Background()); err != nil {
		t.Fatalf("ClearState error: %v", err)
	}
}

func TestResumeUseCase_GetStatusLoadError(t *testing.T) {
	t.Parallel()

	uc := NewResumeUseCase(
		&mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return nil, errors.New("load failed") }},
		&mockStateDetector{},
		NewStateTransitioner(),
		nil,
		newSilentOutputLogger(),
	)

	_, err := uc.GetStatus(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to load state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResumeUseCase_ResumeOptions(t *testing.T) {
	t.Parallel()

	t.Run("clear state", func(t *testing.T) {
		t.Parallel()
		deleted := false
		uc := NewResumeUseCase(
			&mockStateManager{deleteStateFunc: func(ctx context.Context) error { deleted = true; return nil }},
			&mockStateDetector{},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)
		res, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{ClearState: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !deleted || res.Resumed {
			t.Fatalf("clear-state branch not executed")
		}
	})

	t.Run("show status", func(t *testing.T) {
		t.Parallel()
		state := ports.NewUpgradeState("v2", "local", false)
		uc := NewResumeUseCase(
			&mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil }},
			&mockStateDetector{},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)

		res, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{ShowStatus: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Resumed || res.State != state {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("force restart", func(t *testing.T) {
		t.Parallel()
		state := ports.NewUpgradeState("v2", "local", false)
		deleted := false
		uc := NewResumeUseCase(
			&mockStateManager{
				loadStateFunc:   func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil },
				deleteStateFunc: func(ctx context.Context) error { deleted = true; return nil },
			},
			&mockStateDetector{},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)

		res, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{ForceRestart: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !deleted || res.Resumed {
			t.Fatalf("force-restart branch failed")
		}
	})
}

func TestResumeUseCase_LoadAndValidateFailures(t *testing.T) {
	t.Parallel()

	t.Run("corruption error", func(t *testing.T) {
		t.Parallel()
		uc := NewResumeUseCase(
			&mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) {
				return nil, &ports.StateCorruptionError{Reason: "checksum mismatch"}
			}},
			&mockStateDetector{},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)

		_, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{})
		if err == nil || !strings.Contains(err.Error(), "state file is corrupted") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("load error", func(t *testing.T) {
		t.Parallel()
		uc := NewResumeUseCase(
			&mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return nil, errors.New("boom") }},
			&mockStateDetector{},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)

		_, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{})
		if err == nil || !strings.Contains(err.Error(), "failed to load state") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no state", func(t *testing.T) {
		t.Parallel()
		uc := NewResumeUseCase(
			&mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return nil, nil }},
			&mockStateDetector{},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)

		res, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Resumed {
			t.Fatalf("expected no resume")
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		t.Parallel()
		state := ports.NewUpgradeState("v2", "local", false)
		uc := NewResumeUseCase(
			&mockStateManager{
				loadStateFunc:     func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil },
				validateStateFunc: func(state *ports.UpgradeState) error { return errors.New("bad state") },
			},
			&mockStateDetector{},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)

		_, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{})
		if err == nil || !strings.Contains(err.Error(), "invalid state") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestResumeUseCase_TerminalStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		stage   ports.ResumableStage
		error   string
		message string
	}{
		{name: "proposal rejected", stage: ports.ResumableStageProposalRejected, message: "terminal state: ProposalRejected"},
		{name: "failed", stage: ports.ResumableStageFailed, error: "vote failed", message: "terminal state: Failed"},
		{name: "completed", stage: ports.ResumableStageCompleted, message: "terminal state: Completed"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state := ports.NewUpgradeState("v2", "local", false)
			state.Stage = tt.stage
			state.Error = tt.error
			uc := NewResumeUseCase(
				&mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil }},
				&mockStateDetector{},
				NewStateTransitioner(),
				nil,
				newSilentOutputLogger(),
			)

			res, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Resumed {
				t.Fatalf("expected non-resumed result")
			}
			if !strings.Contains(res.Message, tt.message) {
				t.Fatalf("message = %q, expected to contain %q", res.Message, tt.message)
			}
		})
	}
}

func TestResumeUseCase_ResumeFromOverride(t *testing.T) {
	t.Parallel()

	t.Run("invalid transition", func(t *testing.T) {
		t.Parallel()
		state := ports.NewUpgradeState("v2", "local", false)
		tr := &mockTransitioner{canTransitionFunc: func(from, to ports.ResumableStage) bool { return false }}
		uc := NewResumeUseCase(
			&mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil }},
			&mockStateDetector{},
			tr,
			nil,
			newSilentOutputLogger(),
		)

		_, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{ResumeFrom: ports.ResumableStageVoting})
		if err == nil || !strings.Contains(err.Error(), "cannot resume from") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("override save failure", func(t *testing.T) {
		t.Parallel()
		state := ports.NewUpgradeState("v2", "local", false)
		tr := &mockTransitioner{canTransitionFunc: func(from, to ports.ResumableStage) bool { return true }}
		uc := NewResumeUseCase(
			&mockStateManager{
				loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil },
				saveStateFunc: func(ctx context.Context, state *ports.UpgradeState) error { return errors.New("save failed") },
			},
			&mockStateDetector{},
			tr,
			nil,
			newSilentOutputLogger(),
		)

		_, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{}, ports.ResumeOptions{ResumeFrom: ports.ResumableStageProposalSubmitted})
		if err == nil || !strings.Contains(err.Error(), "failed to save state after override") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestResumeUseCase_Reconcile(t *testing.T) {
	t.Parallel()

	t.Run("nil state", func(t *testing.T) {
		t.Parallel()
		uc := NewResumeUseCase(
			&mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return nil, nil }},
			&mockStateDetector{},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)
		state, err := uc.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state != nil {
			t.Fatalf("expected nil state")
		}
	})

	t.Run("detect stage and save", func(t *testing.T) {
		t.Parallel()
		state := ports.NewUpgradeState("v2", "local", false)
		saved := false
		uc := NewResumeUseCase(
			&mockStateManager{
				loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil },
				saveStateFunc: func(ctx context.Context, s *ports.UpgradeState) error { saved = true; return nil },
			},
			&mockStateDetector{detectCurrentStageFunc: func(ctx context.Context, s *ports.UpgradeState) (ports.ResumableStage, error) {
				return ports.ResumableStageProposalSubmitted, nil
			}},
			NewStateTransitioner(),
			nil,
			newSilentOutputLogger(),
		)

		result, err := uc.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Stage != ports.ResumableStageProposalSubmitted || !saved {
			t.Fatalf("unexpected reconcile result: stage=%s saved=%v", result.Stage, saved)
		}
	})
}

func TestResumeUseCase_ResumeExecutesWorkflow(t *testing.T) {
	state := ports.NewUpgradeState("v2", "local", true)
	state.Stage = ports.ResumableStageVerifyingResume

	mgr := &mockStateManager{
		loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil },
	}

	rpc := &mockRPCClient{}
	heights := []int64{10, 11}
	rpc.getBlockHeightFunc = func(ctx context.Context) (int64, error) {
		h := heights[0]
		heights = heights[1:]
		return h, nil
	}

	execUC := &ExecuteUpgradeUseCase{
		rpcClient:  rpc,
		devnetRepo: &mockDevnetRepo{},
		logger:     &testLogger{},
	}

	resExec := &ResumableExecuteUpgradeUseCase{
		executeUC:    execUC,
		stateManager: mgr,
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	uc := NewResumeUseCase(mgr, &mockStateDetector{}, NewStateTransitioner(), resExec, newSilentOutputLogger())

	res, err := uc.Resume(context.Background(), dto.ExecuteUpgradeInput{SkipGovernance: true, Mode: types.ExecutionModeLocal}, ports.ResumeOptions{})
	if err != nil {
		t.Fatalf("expected successful resume, got error: %v", err)
	}
	if !res.Resumed || res.UpgradeOutput == nil || !res.UpgradeOutput.Success {
		t.Fatalf("unexpected resume result: %+v", res)
	}
}
