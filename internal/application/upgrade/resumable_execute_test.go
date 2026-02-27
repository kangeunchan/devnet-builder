package upgrade

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/types"
)

func TestResumableExecuteUpgradeUseCase_Execute_InitialStateSaveFailure(t *testing.T) {
	t.Parallel()

	uc := &ResumableExecuteUpgradeUseCase{
		stateManager: &mockStateManager{saveStateFunc: func(ctx context.Context, state *ports.UpgradeState) error {
			return errors.New("save failed")
		}},
		transitioner: &mockTransitioner{},
		logger:       &testLogger{},
	}

	_, err := uc.Execute(context.Background(), dto.ExecuteUpgradeInput{UpgradeName: "v2", Mode: types.ExecutionModeLocal, SkipGovernance: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to save initial state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewResumableExecuteUpgradeUseCase(t *testing.T) {
	t.Parallel()

	uc := NewResumableExecuteUpgradeUseCase(
		nil,
		nil,
		nil,
		nil,
		&mockStateManager{},
		NewStateTransitioner(),
		&mockStateDetector{},
		&mockRPCClient{},
		&mockExportUseCase{},
		&mockDevnetRepo{},
		&testLogger{},
	)
	if uc == nil {
		t.Fatalf("expected non-nil use case")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSkipGovCompleted(t *testing.T) {
	t.Parallel()

	deleted := false
	uc := &ResumableExecuteUpgradeUseCase{
		stateManager: &mockStateManager{deleteStateFunc: func(ctx context.Context) error { deleted = true; return nil }},
		logger:       &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", true)
	state.Stage = ports.ResumableStageCompleted
	out, err := uc.executeSkipGovResumable(context.Background(), dto.ExecuteUpgradeInput{}, state, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success || !deleted {
		t.Fatalf("expected completed success and state deletion")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSkipGovUnknownStage(t *testing.T) {
	t.Parallel()

	uc := &ResumableExecuteUpgradeUseCase{logger: &testLogger{}}
	state := ports.NewUpgradeState("v2", "local", true)
	state.Stage = "Unknown"
	_, err := uc.executeSkipGovResumable(context.Background(), dto.ExecuteUpgradeInput{}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "cannot resume from stage") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovTerminalStages(t *testing.T) {
	t.Parallel()

	uc := &ResumableExecuteUpgradeUseCase{logger: &testLogger{}}

	failed := ports.NewUpgradeState("v2", "local", false)
	failed.Stage = ports.ResumableStageFailed
	failed.Error = "vote failed"
	_, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{}, failed, time.Now())
	if err == nil || !strings.Contains(err.Error(), "upgrade previously failed") {
		t.Fatalf("unexpected failed-stage error: %v", err)
	}

	rejected := ports.NewUpgradeState("v2", "local", false)
	rejected.Stage = ports.ResumableStageProposalRejected
	_, err = uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{}, rejected, time.Now())
	if err == nil || !strings.Contains(err.Error(), "proposal was rejected") {
		t.Fatalf("unexpected rejected-stage error: %v", err)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovPreExportFailure(t *testing.T) {
	t.Parallel()

	uc := &ResumableExecuteUpgradeUseCase{
		exportUC: &mockExportUseCase{executeFunc: func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("export fail")
		}},
		logger: &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageInitialized

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{WithExport: true, HomeDir: "/tmp/devnet"}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "pre-upgrade export failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output error to be populated")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovCompleted(t *testing.T) {
	t.Parallel()

	deleted := false
	uc := &ResumableExecuteUpgradeUseCase{
		stateManager: &mockStateManager{deleteStateFunc: func(ctx context.Context) error { deleted = true; return nil }},
		logger:       &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageCompleted

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{}, state, time.Now().Add(-2*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success || !deleted {
		t.Fatalf("expected successful completion path")
	}
}

func TestResumableExecuteUpgradeUseCase_TransitionAndSave(t *testing.T) {
	t.Parallel()

	state := ports.NewUpgradeState("v2", "local", false)

	uc := &ResumableExecuteUpgradeUseCase{
		transitioner: NewStateTransitioner(),
		stateManager: &mockStateManager{},
		logger:       &testLogger{},
	}
	if err := uc.transitionAndSave(context.Background(), state, ports.ResumableStageProposalSubmitted, "ok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ucErr := &ResumableExecuteUpgradeUseCase{
		transitioner: &mockTransitioner{transitionToFunc: func(state *ports.UpgradeState, target ports.ResumableStage, reason string) error {
			return errors.New("bad transition")
		}},
		stateManager: &mockStateManager{},
		logger:       &testLogger{},
	}
	if err := ucErr.transitionAndSave(context.Background(), state, ports.ResumableStageVoting, "x"); err == nil {
		t.Fatalf("expected transition error")
	}

	ucSaveErr := &ResumableExecuteUpgradeUseCase{
		transitioner: NewStateTransitioner(),
		stateManager: &mockStateManager{saveStateFunc: func(ctx context.Context, state *ports.UpgradeState) error { return errors.New("disk full") }},
		logger:       &testLogger{},
	}
	state2 := ports.NewUpgradeState("v2", "local", false)
	if err := ucSaveErr.transitionAndSave(context.Background(), state2, ports.ResumableStageProposalSubmitted, "x"); err == nil || !strings.Contains(err.Error(), "failed to save state") {
		t.Fatalf("unexpected save error: %v", err)
	}
}

func TestResumableExecuteUpgradeUseCase_StateAccessors(t *testing.T) {
	t.Parallel()

	state := ports.NewUpgradeState("v2", "local", false)
	mgr := &mockStateManager{loadStateFunc: func(ctx context.Context) (*ports.UpgradeState, error) { return state, nil }}
	uc := &ResumableExecuteUpgradeUseCase{stateManager: mgr}

	got, err := uc.GetCurrentState(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentState error: %v", err)
	}
	if got != state {
		t.Fatalf("state mismatch")
	}

	if err := uc.ClearState(context.Background()); err != nil {
		t.Fatalf("ClearState error: %v", err)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSwitchBinaryAndVotingHelpers(t *testing.T) {
	state := ports.NewUpgradeState("v2", "local", false)
	state.UpgradeHeight = 100
	state.ProposalID = 10

	pid := 3000
	switchUC := NewSwitchBinaryUseCase(
		&mockDevnetRepo{},
		&mockNodeRepo{loadAllFunc: func(ctx context.Context, homeDir string) ([]*ports.NodeMetadata, error) {
			return []*ports.NodeMetadata{
				{Index: 0, Name: "node0", HomeDir: "/tmp/node0", PID: &pid},
				{Index: 1, Name: "node1", HomeDir: "/tmp/node1"},
			}, nil
		}},
		&mockProcessExecutor{},
		&mockBinaryCache{},
		&testLogger{},
	)

	voteUC := NewVoteUseCase(
		&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
			return &ports.DevnetMetadata{
				Status:         ports.StateRunning,
				NumValidators:  0,
				ExecutionMode:  types.ExecutionModeLocal,
				CurrentVersion: "v1.0.0",
				BinaryName:     "stabled",
			}, nil
		}},
		&mockRPCClient{getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
			return &ports.Proposal{Status: ports.ProposalStatusVoting}, nil
		}},
		&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
			return []ports.ValidatorKey{}, nil
		}},
		&testLogger{},
	)

	uc := &ResumableExecuteUpgradeUseCase{
		switchUC:      switchUC,
		voteUC:        voteUC,
		stateManager:  &mockStateManager{},
		transitioner:  NewStateTransitioner(),
		stateDetector: &mockStateDetector{},
		logger:        &testLogger{},
	}

	switchOut, err := uc.executeSwitchBinary(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:      "/tmp/devnet",
		TargetBinary: "/tmp/new-stabled",
		Mode:         types.ExecutionModeLocal,
		CacheRef:     "cache-ref",
	}, state)
	if err != nil {
		t.Fatalf("executeSwitchBinary error: %v", err)
	}
	if switchOut.NewBinary == "" {
		t.Fatalf("expected new binary path")
	}
	if len(state.NodeSwitches) != 2 {
		t.Fatalf("expected node switch tracking, got %d", len(state.NodeSwitches))
	}

	voteOut, err := uc.executeVoting(context.Background(), dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state)
	if err != nil {
		t.Fatalf("executeVoting error: %v", err)
	}
	if voteOut.TotalVoters != 0 || voteOut.VotesCast != 0 {
		t.Fatalf("unexpected vote output: %+v", voteOut)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSkipGovResumableFromSwitching(t *testing.T) {
	heights := []int64{10, 11}

	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			}},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		switchUC:     NewSwitchBinaryUseCase(&mockDevnetRepo{}, &mockNodeRepo{}, &mockProcessExecutor{}, &mockBinaryCache{}, &testLogger{}),
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", true)
	state.Stage = ports.ResumableStageSwitchingBinary

	out, err := uc.executeSkipGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:      "/tmp/devnet",
		Mode:         types.ExecutionModeLocal,
		TargetBinary: "/tmp/new-stabled",
		CacheRef:     "cache-ref",
	}, state, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success output")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_VotingToWaitError(t *testing.T) {
	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageProposalSubmitted
	state.ProposalID = 15
	state.UpgradeHeight = 100

	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient:  &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 0, errors.New("height down") }},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		voteUC: NewVoteUseCase(
			&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
				return &ports.DevnetMetadata{
					Status:         ports.StateRunning,
					NumValidators:  0,
					ExecutionMode:  types.ExecutionModeLocal,
					CurrentVersion: "v1.0.0",
					BinaryName:     "stabled",
				}, nil
			}},
			&mockRPCClient{getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
				return &ports.Proposal{Status: ports.ProposalStatusVoting}, nil
			}},
			&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
				return []ports.ValidatorKey{}, nil
			}},
			&testLogger{},
		),
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "failed to get block height") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output with error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_FromInitializedUntilWaitError(t *testing.T) {
	teardown := startFakeEthRPCServer(t, 22)
	defer teardown()

	key := testValidatorKey(t)
	keyLoader := &mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
		return []ports.ValidatorKey{key}, nil
	}}

	proposeUC := NewProposeUseCase(
		&mockDevnetRepo{},
		&mockRPCClient{},
		keyLoader,
		&testLogger{},
	)
	voteUC := NewVoteUseCase(
		&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
			return &ports.DevnetMetadata{
				Status:         ports.StateRunning,
				NumValidators:  1,
				ExecutionMode:  types.ExecutionModeLocal,
				CurrentVersion: "v1.0.0",
				BinaryName:     "stabled",
			}, nil
		}},
		&mockRPCClient{getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
			return &ports.Proposal{Status: ports.ProposalStatusVoting}, nil
		}},
		keyLoader,
		&testLogger{},
	)

	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient:  &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 0, errors.New("height down") }},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		proposeUC: proposeUC,
		voteUC:    voteUC,
		exportUC: &mockExportUseCase{executeFunc: func(ctx context.Context, input interface{}) (interface{}, error) {
			return &dto.ExportOutput{ExportPath: "/tmp/pre-upgrade.json"}, nil
		}},
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageInitialized

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:       "/tmp/devnet",
		UpgradeName:   "v2.0.0",
		UpgradeHeight: 100,
		VotingPeriod:  30 * time.Second,
		WithExport:    true,
	}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "failed to get block height") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.PreGenesisPath == "" || out.Error == nil {
		t.Fatalf("expected output with pre-export path and error: %+v", out)
	}
	if state.Stage != ports.ResumableStageFailed {
		t.Fatalf("expected failed stage, got %s", state.Stage)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_FromVerifyingResumeSuccess(t *testing.T) {
	heights := []int64{200, 201}
	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			}},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		exportUC: &mockExportUseCase{executeFunc: func(ctx context.Context, input interface{}) (interface{}, error) {
			return &dto.ExportOutput{ExportPath: "/tmp/post-upgrade.json"}, nil
		}},
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageVerifyingResume
	state.ProposalID = 10
	state.UpgradeHeight = 100

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:    "/tmp/devnet",
		WithExport: true,
	}, state, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success || out.PostGenesisPath == "" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSkipGovResumable_SwitchError(t *testing.T) {
	uc := &ResumableExecuteUpgradeUseCase{
		switchUC: NewSwitchBinaryUseCase(
			&mockDevnetRepo{},
			&mockNodeRepo{},
			&mockProcessExecutor{},
			&mockBinaryCache{setActiveFunc: func(ref string) error { return errors.New("activate failed") }},
			&testLogger{},
		),
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", true)
	state.Stage = ports.ResumableStageSwitchingBinary

	out, err := uc.executeSkipGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:      "/tmp/devnet",
		Mode:         types.ExecutionModeLocal,
		TargetBinary: "/tmp/new-stabled",
		CacheRef:     "cache-ref",
	}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "failed to activate binary") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteVoting_WithValidatorTx(t *testing.T) {
	teardown := startFakeEthRPCServer(t, 33)
	defer teardown()

	key := testValidatorKey(t)
	voteUC := NewVoteUseCase(
		&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
			return &ports.DevnetMetadata{
				Status:         ports.StateRunning,
				NumValidators:  1,
				ExecutionMode:  types.ExecutionModeLocal,
				CurrentVersion: "v1.0.0",
				BinaryName:     "stabled",
			}, nil
		}},
		&mockRPCClient{getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
			return &ports.Proposal{Status: ports.ProposalStatusVoting}, nil
		}},
		&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
			return []ports.ValidatorKey{key}, nil
		}},
		&testLogger{},
	)

	state := ports.NewUpgradeState("v2", "local", false)
	state.ProposalID = 33
	uc := &ResumableExecuteUpgradeUseCase{
		voteUC:       voteUC,
		stateManager: &mockStateManager{},
		logger:       &testLogger{},
	}

	out, err := uc.executeVoting(context.Background(), dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.VotesCast != 1 || len(state.ValidatorVotes) != 1 || !state.ValidatorVotes[0].Voted {
		t.Fatalf("unexpected vote tracking: out=%+v state=%+v", out, state.ValidatorVotes)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSwitchBinary_UpdatesExistingNodeEntry(t *testing.T) {
	pid := 4000
	switchUC := NewSwitchBinaryUseCase(
		&mockDevnetRepo{},
		&mockNodeRepo{loadAllFunc: func(ctx context.Context, homeDir string) ([]*ports.NodeMetadata, error) {
			return []*ports.NodeMetadata{{Index: 0, Name: "node0", HomeDir: "/tmp/node0", PID: &pid}}, nil
		}},
		&mockProcessExecutor{},
		&mockBinaryCache{},
		&testLogger{},
	)
	state := ports.NewUpgradeState("v2", "local", false)
	state.UpgradeHeight = 100
	state.NodeSwitches = []ports.NodeSwitchState{{NodeName: "node0", Switched: false}}

	uc := &ResumableExecuteUpgradeUseCase{
		switchUC:      switchUC,
		stateManager:  &mockStateManager{},
		transitioner:  NewStateTransitioner(),
		stateDetector: &mockStateDetector{},
		logger:        &testLogger{},
	}

	_, err := uc.executeSwitchBinary(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:      "/tmp/devnet",
		TargetBinary: "/tmp/new-stabled",
		Mode:         types.ExecutionModeLocal,
		CacheRef:     "cache-ref",
	}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.NodeSwitches) != 1 || !state.NodeSwitches[0].Switched {
		t.Fatalf("existing node switch entry was not updated: %+v", state.NodeSwitches)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_VotingMismatch(t *testing.T) {
	teardown := startFakeEthRPCServer(t, 44)
	defer teardown()

	valid := testValidatorKey(t)
	invalid := ports.ValidatorKey{
		Name:       "validator1",
		HexAddress: valid.HexAddress,
		PrivateKey: "not-a-private-key",
	}

	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageVoting
	state.ProposalID = 44
	state.UpgradeHeight = 100

	uc := &ResumableExecuteUpgradeUseCase{
		voteUC: NewVoteUseCase(
			&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
				return &ports.DevnetMetadata{
					Status:         ports.StateRunning,
					NumValidators:  2,
					ExecutionMode:  types.ExecutionModeLocal,
					CurrentVersion: "v1.0.0",
					BinaryName:     "stabled",
				}, nil
			}},
			&mockRPCClient{getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
				return &ports.Proposal{Status: ports.ProposalStatusVoting}, nil
			}},
			&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
				return []ports.ValidatorKey{valid, invalid}, nil
			}},
			&testLogger{},
		),
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "not all votes cast") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_ChainHaltError(t *testing.T) {
	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageChainHalted
	state.ProposalID = 50
	state.UpgradeHeight = 100

	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient:  &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 200, nil }},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "upgrade proposal may have failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_VerifyError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageVerifyingResume
	state.ProposalID = 60
	state.UpgradeHeight = 100

	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient:  &mockRPCClient{},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	out, err := uc.executeWithGovResumable(ctx, dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state, time.Now())
	if err == nil {
		t.Fatalf("expected verify error")
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_UnknownStage(t *testing.T) {
	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = "Unknown"
	uc := &ResumableExecuteUpgradeUseCase{logger: &testLogger{}}

	_, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "cannot resume from stage") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_InitializedProposeError(t *testing.T) {
	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageInitialized

	uc := &ResumableExecuteUpgradeUseCase{
		proposeUC: NewProposeUseCase(
			&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
				return &ports.DevnetMetadata{Status: ports.StateStopped}, nil
			}},
			&mockRPCClient{},
			&mockValidatorKeyLoader{},
			&testLogger{},
		),
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "devnet is not running") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Error == nil || state.Stage != ports.ResumableStageFailed {
		t.Fatalf("expected failed transition and output error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_WaitSuccessThenChainHaltError(t *testing.T) {
	heights := []int64{100, 200}
	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageWaitingForHeight
	state.ProposalID = 70
	state.UpgradeHeight = 100

	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			}},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "upgrade proposal may have failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_SwitchError(t *testing.T) {
	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageSwitchingBinary
	state.ProposalID = 80
	state.UpgradeHeight = 100

	uc := &ResumableExecuteUpgradeUseCase{
		switchUC: NewSwitchBinaryUseCase(
			&mockDevnetRepo{},
			&mockNodeRepo{},
			&mockProcessExecutor{},
			&mockBinaryCache{setActiveFunc: func(ref string) error { return errors.New("activate failed") }},
			&testLogger{},
		),
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:      "/tmp/devnet",
		Mode:         types.ExecutionModeLocal,
		TargetBinary: "/tmp/new-stabled",
		CacheRef:     "cache-ref",
	}, state, time.Now())
	if err == nil || !strings.Contains(err.Error(), "failed to activate binary") {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteWithGovResumable_PostExportFailureNonFatal(t *testing.T) {
	heights := []int64{300, 301}
	state := ports.NewUpgradeState("v2", "local", false)
	state.Stage = ports.ResumableStageVerifyingResume
	state.ProposalID = 90
	state.UpgradeHeight = 100

	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			}},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		exportUC: &mockExportUseCase{executeFunc: func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("post export failed")
		}},
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	out, err := uc.executeWithGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:    "/tmp/devnet",
		WithExport: true,
	}, state, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success despite post-export failure")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSkipGovResumable_FromInitializedSuccess(t *testing.T) {
	heights := []int64{500, 501}
	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			}},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		switchUC:      NewSwitchBinaryUseCase(&mockDevnetRepo{}, &mockNodeRepo{}, &mockProcessExecutor{}, &mockBinaryCache{}, &testLogger{}),
		stateManager:  &mockStateManager{},
		transitioner:  NewStateTransitioner(),
		stateDetector: &mockStateDetector{},
		logger:        &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", true)
	state.Stage = ports.ResumableStageInitialized

	out, err := uc.executeSkipGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:      "/tmp/devnet",
		Mode:         types.ExecutionModeLocal,
		TargetBinary: "/tmp/new-stabled",
		CacheRef:     "cache-ref",
	}, state, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected successful skip-gov resumable output")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSkipGovResumable_VerifyError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			rpcClient:  &mockRPCClient{},
			devnetRepo: &mockDevnetRepo{},
			logger:     &testLogger{},
		},
		stateManager: &mockStateManager{},
		transitioner: NewStateTransitioner(),
		logger:       &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", true)
	state.Stage = ports.ResumableStageVerifyingResume

	out, err := uc.executeSkipGovResumable(ctx, dto.ExecuteUpgradeInput{HomeDir: "/tmp/devnet"}, state, time.Now())
	if err == nil {
		t.Fatalf("expected verify error")
	}
	if out == nil || out.Error == nil {
		t.Fatalf("expected output error")
	}
}

func TestResumableExecuteUpgradeUseCase_ExecuteSkipGovResumable_CompletedWithTargetVersion(t *testing.T) {
	uc := &ResumableExecuteUpgradeUseCase{
		executeUC: &ExecuteUpgradeUseCase{
			devnetRepo: &mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
				return nil, errors.New("load fail")
			}},
			logger: &testLogger{},
		},
		stateManager: &mockStateManager{},
		logger:       &testLogger{},
	}

	state := ports.NewUpgradeState("v2", "local", true)
	state.Stage = ports.ResumableStageCompleted

	out, err := uc.executeSkipGovResumable(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:       "/tmp/devnet",
		TargetVersion: "v2.0.0",
	}, state, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success output")
	}
}
