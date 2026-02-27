package upgrade

import (
	"testing"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

func TestStateTransitioner_CanTransition(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()

	tests := []struct {
		name string
		from ports.ResumableStage
		to   ports.ResumableStage
		want bool
	}{
		{name: "initialized to proposal", from: ports.ResumableStageInitialized, to: ports.ResumableStageProposalSubmitted, want: true},
		{name: "initialized to failed", from: ports.ResumableStageInitialized, to: ports.ResumableStageFailed, want: true},
		{name: "completed to failed invalid", from: ports.ResumableStageCompleted, to: ports.ResumableStageFailed, want: false},
		{name: "unknown source invalid", from: "Unknown", to: ports.ResumableStageFailed, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tr.CanTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestStateTransitioner_TransitionTo(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()
	state := ports.NewUpgradeState("v2", "local", false)

	if err := tr.TransitionTo(state, ports.ResumableStageProposalSubmitted, "proposal submitted"); err != nil {
		t.Fatalf("TransitionTo returned error: %v", err)
	}

	if state.Stage != ports.ResumableStageProposalSubmitted {
		t.Fatalf("state.Stage = %s, want %s", state.Stage, ports.ResumableStageProposalSubmitted)
	}
	if len(state.StageHistory) != 2 {
		t.Fatalf("history length = %d, want 2", len(state.StageHistory))
	}
	last := state.StageHistory[len(state.StageHistory)-1]
	if last.From != ports.ResumableStageInitialized || last.To != ports.ResumableStageProposalSubmitted {
		t.Fatalf("unexpected transition history: %+v", last)
	}
	if last.Reason != "proposal submitted" {
		t.Fatalf("reason = %q, want %q", last.Reason, "proposal submitted")
	}
	if state.UpdatedAt.Before(state.CreatedAt) {
		t.Fatalf("updatedAt should be >= createdAt")
	}
}

func TestStateTransitioner_TransitionToFailedSetsError(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()
	state := ports.NewUpgradeState("v2", "local", false)

	if err := tr.TransitionTo(state, ports.ResumableStageFailed, "switch failed"); err != nil {
		t.Fatalf("TransitionTo returned error: %v", err)
	}
	if state.Error != "switch failed" {
		t.Fatalf("state.Error = %q, want %q", state.Error, "switch failed")
	}
}

func TestStateTransitioner_TransitionToInvalid(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()
	state := ports.NewUpgradeState("v2", "local", false)

	err := tr.TransitionTo(state, ports.ResumableStageCompleted, "skip")
	if err == nil {
		t.Fatalf("expected invalid transition error")
	}
}

func TestStateTransitioner_TransitionToNilState(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()
	err := tr.TransitionTo(nil, ports.ResumableStageCompleted, "nil")
	if err == nil {
		t.Fatalf("expected error for nil state")
	}
}

func TestStateTransitioner_GetValidTransitionsReturnsCopy(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()
	transitions := tr.GetValidTransitions(ports.ResumableStageInitialized)
	if len(transitions) == 0 {
		t.Fatalf("expected transitions")
	}
	transitions[0] = ports.ResumableStageCompleted

	transitions2 := tr.GetValidTransitions(ports.ResumableStageInitialized)
	if transitions2[0] == ports.ResumableStageCompleted {
		t.Fatalf("expected copy, got shared backing array")
	}
}

func TestStateTransitioner_IsGovernanceRequired(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()

	if !tr.IsGovernanceRequired(&ports.UpgradeState{SkipGovernance: false}) {
		t.Fatalf("expected governance required")
	}
	if tr.IsGovernanceRequired(&ports.UpgradeState{SkipGovernance: true}) {
		t.Fatalf("expected governance not required")
	}
}

func TestStateTransitioner_GetNextStages(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()

	govCases := []struct {
		from ports.ResumableStage
		to   ports.ResumableStage
	}{
		{from: ports.ResumableStageInitialized, to: ports.ResumableStageProposalSubmitted},
		{from: ports.ResumableStageProposalSubmitted, to: ports.ResumableStageVoting},
		{from: ports.ResumableStageVoting, to: ports.ResumableStageWaitingForHeight},
		{from: ports.ResumableStageWaitingForHeight, to: ports.ResumableStageChainHalted},
		{from: ports.ResumableStageChainHalted, to: ports.ResumableStageSwitchingBinary},
		{from: ports.ResumableStageSwitchingBinary, to: ports.ResumableStageVerifyingResume},
		{from: ports.ResumableStageVerifyingResume, to: ports.ResumableStageCompleted},
	}
	for _, c := range govCases {
		if got := tr.GetNextStageForGovPath(c.from); got != c.to {
			t.Fatalf("gov next(%s)=%s, want %s", c.from, got, c.to)
		}
	}
	if got := tr.GetNextStageForGovPath("unknown"); got != "" {
		t.Fatalf("expected empty stage, got %s", got)
	}

	if got := tr.GetNextStageForSkipGovPath(ports.ResumableStageInitialized); got != ports.ResumableStageSwitchingBinary {
		t.Fatalf("skip-gov next = %s", got)
	}
	if got := tr.GetNextStageForSkipGovPath(ports.ResumableStageVerifyingResume); got != ports.ResumableStageCompleted {
		t.Fatalf("skip-gov final next = %s", got)
	}
	if got := tr.GetNextStageForSkipGovPath(ports.ResumableStageVoting); got != "" {
		t.Fatalf("expected empty stage, got %s", got)
	}
}

func TestStateTransitioner_TransitionHistoryTimestamp(t *testing.T) {
	t.Parallel()

	tr := NewStateTransitioner()
	state := ports.NewUpgradeState("v2", "local", false)
	before := time.Now().Add(-time.Second)

	if err := tr.TransitionTo(state, ports.ResumableStageProposalSubmitted, "ok"); err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	last := state.StageHistory[len(state.StageHistory)-1]
	if last.Timestamp.Before(before) {
		t.Fatalf("expected transition timestamp to be current")
	}
}
