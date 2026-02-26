package upgrade

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

type upgradeTestLogger struct{}

func (upgradeTestLogger) Info(string, ...interface{})    {}
func (upgradeTestLogger) Warn(string, ...interface{})    {}
func (upgradeTestLogger) Error(string, ...interface{})   {}
func (upgradeTestLogger) Debug(string, ...interface{})   {}
func (upgradeTestLogger) Success(string, ...interface{}) {}
func (upgradeTestLogger) Print(string, ...interface{})   {}
func (upgradeTestLogger) Println(string, ...interface{}) {}
func (upgradeTestLogger) SetVerbose(bool)                {}
func (upgradeTestLogger) IsVerbose() bool                { return false }
func (upgradeTestLogger) Writer() io.Writer              { return io.Discard }
func (upgradeTestLogger) ErrWriter() io.Writer           { return io.Discard }

func newResumableUCTestHarness(ops *resumableUpgradeOps) *ResumableExecuteUpgradeUseCase {
	if ops.transitionAndSave == nil {
		ops.transitionAndSave = func(_ context.Context, state *ports.UpgradeState, target ports.ResumableStage, _ string) error {
			state.Stage = target
			return nil
		}
	}
	return &ResumableExecuteUpgradeUseCase{
		logger: upgradeTestLogger{},
		ops:    ops,
	}
}

func TestGovResumableStageHandlersCoverage(t *testing.T) {
	uc := newResumableUCTestHarness(&resumableUpgradeOps{})
	handlers := uc.govResumableStageHandlers()

	expected := []ports.ResumableStage{
		ports.ResumableStageInitialized,
		ports.ResumableStageProposalSubmitted,
		ports.ResumableStageVoting,
		ports.ResumableStageWaitingForHeight,
		ports.ResumableStageChainHalted,
		ports.ResumableStageSwitchingBinary,
		ports.ResumableStageVerifyingResume,
	}

	if len(handlers) != len(expected) {
		t.Fatalf("handler count mismatch: got %d want %d", len(handlers), len(expected))
	}

	for _, stage := range expected {
		if _, ok := handlers[stage]; !ok {
			t.Fatalf("missing handler for stage %s", stage)
		}
	}
}

func TestHandleGovStageInitialized(t *testing.T) {
	uc := newResumableUCTestHarness(&resumableUpgradeOps{
		executeProposal: func(context.Context, dto.ProposeInput) (*dto.ProposeOutput, error) {
			return &dto.ProposeOutput{ProposalID: 42, UpgradeHeight: 777}, nil
		},
	})
	state := ports.NewUpgradeState("upgrade", "local", false)
	output := &dto.ExecuteUpgradeOutput{}

	outcome := uc.handleGovStageInitialized(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:     "/tmp/home",
		UpgradeName: "upgrade",
	}, state, output)

	if outcome.err != nil {
		t.Fatalf("unexpected error: %v", outcome.err)
	}
	if state.Stage != ports.ResumableStageProposalSubmitted {
		t.Fatalf("stage mismatch: got %s", state.Stage)
	}
	if output.ProposalID != 42 || output.UpgradeHeight != 777 {
		t.Fatalf("unexpected proposal output: %+v", output)
	}
}

func TestHandleGovStageVoting_PartialVotesMarksFailed(t *testing.T) {
	transitions := make([]ports.ResumableStage, 0, 1)
	uc := newResumableUCTestHarness(&resumableUpgradeOps{
		executeVote: func(context.Context, dto.ExecuteUpgradeInput, *ports.UpgradeState) (*dto.VoteOutput, error) {
			return &dto.VoteOutput{VotesCast: 1, TotalVoters: 2}, nil
		},
		transitionAndSave: func(_ context.Context, state *ports.UpgradeState, target ports.ResumableStage, _ string) error {
			transitions = append(transitions, target)
			state.Stage = target
			return nil
		},
	})
	state := ports.NewUpgradeState("upgrade", "local", false)
	state.Stage = ports.ResumableStageVoting
	state.ProposalID = 10
	state.UpgradeHeight = 1234
	output := &dto.ExecuteUpgradeOutput{}

	outcome := uc.handleGovStageVoting(context.Background(), dto.ExecuteUpgradeInput{}, state, output)
	if outcome.err == nil {
		t.Fatal("expected error for partial votes")
	}
	if !outcome.preserveOutputOnError {
		t.Fatal("expected preserveOutputOnError=true on stage execution failure")
	}
	if output.Error == nil || !strings.Contains(output.Error.Error(), "not all votes cast") {
		t.Fatalf("unexpected output error: %v", output.Error)
	}
	if len(transitions) != 1 || transitions[0] != ports.ResumableStageFailed {
		t.Fatalf("unexpected transitions: %+v", transitions)
	}
}

func TestHandleGovStageSwitchingBinary(t *testing.T) {
	uc := newResumableUCTestHarness(&resumableUpgradeOps{
		executeSwitchBinary: func(context.Context, dto.ExecuteUpgradeInput, *ports.UpgradeState) (*dto.SwitchBinaryOutput, error) {
			return &dto.SwitchBinaryOutput{NewBinary: "/tmp/newd"}, nil
		},
	})
	state := ports.NewUpgradeState("upgrade", "local", false)
	state.Stage = ports.ResumableStageSwitchingBinary
	state.ProposalID = 9
	state.UpgradeHeight = 999
	output := &dto.ExecuteUpgradeOutput{}

	outcome := uc.handleGovStageSwitchingBinary(context.Background(), dto.ExecuteUpgradeInput{}, state, output)
	if outcome.err != nil {
		t.Fatalf("unexpected error: %v", outcome.err)
	}
	if state.Stage != ports.ResumableStageVerifyingResume {
		t.Fatalf("stage mismatch: got %s", state.Stage)
	}
	if output.NewBinary != "/tmp/newd" {
		t.Fatalf("new binary mismatch: %s", output.NewBinary)
	}
}

func TestHandleGovStageVerifyingResume_WithExport(t *testing.T) {
	exportCalls := 0
	uc := newResumableUCTestHarness(&resumableUpgradeOps{
		verifyChainResumed: func(context.Context, string) (int64, error) {
			return 2048, nil
		},
		executeExport: func(context.Context, dto.ExportInput) (interface{}, error) {
			exportCalls++
			return &dto.ExportOutput{ExportPath: "/tmp/post-export.json"}, nil
		},
	})
	state := ports.NewUpgradeState("upgrade", "local", false)
	state.Stage = ports.ResumableStageVerifyingResume
	state.ProposalID = 11
	state.UpgradeHeight = 2047
	output := &dto.ExecuteUpgradeOutput{}

	outcome := uc.handleGovStageVerifyingResume(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:     "/tmp/home",
		GenesisDir:  "/tmp/genesis",
		WithExport:  true,
		UpgradeName: "upgrade",
	}, state, output)
	if outcome.err != nil {
		t.Fatalf("unexpected error: %v", outcome.err)
	}
	if state.Stage != ports.ResumableStageCompleted {
		t.Fatalf("stage mismatch: got %s", state.Stage)
	}
	if output.PostUpgradeHeight != 2048 {
		t.Fatalf("post height mismatch: got %d", output.PostUpgradeHeight)
	}
	if output.PostGenesisPath != "/tmp/post-export.json" {
		t.Fatalf("post export path mismatch: %s", output.PostGenesisPath)
	}
	if exportCalls != 1 {
		t.Fatalf("unexpected export calls: %d", exportCalls)
	}
}

func TestExecuteWithGovResumable_StateMachineLoop(t *testing.T) {
	var transitions []ports.ResumableStage
	exportCalls := 0
	updateCalls := 0
	deleteCalls := 0

	uc := newResumableUCTestHarness(&resumableUpgradeOps{
		executeProposal: func(context.Context, dto.ProposeInput) (*dto.ProposeOutput, error) {
			return &dto.ProposeOutput{ProposalID: 7, UpgradeHeight: 150}, nil
		},
		executeVote: func(context.Context, dto.ExecuteUpgradeInput, *ports.UpgradeState) (*dto.VoteOutput, error) {
			return &dto.VoteOutput{VotesCast: 4, TotalVoters: 4}, nil
		},
		waitForUpgradeHeight: func(context.Context, int64) error { return nil },
		waitForChainHalt:     func(context.Context, int64) error { return nil },
		executeSwitchBinary: func(context.Context, dto.ExecuteUpgradeInput, *ports.UpgradeState) (*dto.SwitchBinaryOutput, error) {
			return &dto.SwitchBinaryOutput{NewBinary: "/tmp/new-binary"}, nil
		},
		verifyChainResumed: func(context.Context, string) (int64, error) { return 151, nil },
		executeExport: func(context.Context, dto.ExportInput) (interface{}, error) {
			exportCalls++
			if exportCalls == 1 {
				return &dto.ExportOutput{ExportPath: "/tmp/pre.json"}, nil
			}
			return &dto.ExportOutput{ExportPath: "/tmp/post.json"}, nil
		},
		updateCurrentVersion: func(context.Context, string, string) error {
			updateCalls++
			return nil
		},
		deleteState: func(context.Context) error {
			deleteCalls++
			return nil
		},
		transitionAndSave: func(_ context.Context, state *ports.UpgradeState, target ports.ResumableStage, _ string) error {
			transitions = append(transitions, target)
			state.Stage = target
			return nil
		},
	})

	state := ports.NewUpgradeState("upgrade", "docker", false)
	output, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:       "/tmp/home",
		UpgradeName:   "upgrade",
		TargetVersion: "v2.0.0",
		GenesisDir:    "/tmp/genesis",
		WithExport:    true,
	}, state, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Success {
		t.Fatal("expected success output")
	}
	if output.PreGenesisPath != "/tmp/pre.json" || output.PostGenesisPath != "/tmp/post.json" {
		t.Fatalf("unexpected export paths: pre=%s post=%s", output.PreGenesisPath, output.PostGenesisPath)
	}
	if output.NewBinary != "/tmp/new-binary" || output.PostUpgradeHeight != 151 {
		t.Fatalf("unexpected output values: %+v", output)
	}
	if updateCalls != 1 || deleteCalls != 1 {
		t.Fatalf("unexpected finalize calls update=%d delete=%d", updateCalls, deleteCalls)
	}
	if len(transitions) != 7 {
		t.Fatalf("unexpected transitions length: %d (%+v)", len(transitions), transitions)
	}
}
