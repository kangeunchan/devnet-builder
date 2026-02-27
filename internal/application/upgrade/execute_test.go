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

func TestExecuteUpgradeUseCase_ExecuteSkipGov(t *testing.T) {
	t.Parallel()

	cacheSetActiveCalled := false
	switchUC := NewSwitchBinaryUseCase(
		&mockDevnetRepo{},
		&mockNodeRepo{},
		&mockProcessExecutor{},
		&mockBinaryCache{
			setActiveFunc: func(ref string) error {
				cacheSetActiveCalled = true
				if ref != "cache-ref" {
					t.Fatalf("unexpected cache ref: %s", ref)
				}
				return nil
			},
		},
		&testLogger{},
	)

	rpc := &mockRPCClient{}
	heights := []int64{100, 101}
	rpc.getBlockHeightFunc = func(ctx context.Context) (int64, error) {
		h := heights[0]
		heights = heights[1:]
		return h, nil
	}

	uc := NewExecuteUpgradeUseCase(
		nil,
		nil,
		switchUC,
		&mockExportUseCase{},
		rpc,
		&mockDevnetRepo{},
		nil,
		&testLogger{},
	)

	out, err := uc.Execute(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:        "/tmp/devnet",
		SkipGovernance: true,
		TargetBinary:   "/tmp/stabled",
		CacheRef:       "cache-ref",
		Mode:           types.ExecutionModeLocal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success output")
	}
	if !cacheSetActiveCalled {
		t.Fatalf("expected SetActive to be called")
	}
}

func TestExecuteUpgradeUseCase_ExecuteSkipGovSwitchError(t *testing.T) {
	t.Parallel()

	switchUC := NewSwitchBinaryUseCase(
		&mockDevnetRepo{},
		&mockNodeRepo{},
		&mockProcessExecutor{},
		&mockBinaryCache{},
		&testLogger{},
	)

	uc := NewExecuteUpgradeUseCase(
		nil,
		nil,
		switchUC,
		&mockExportUseCase{},
		&mockRPCClient{},
		&mockDevnetRepo{},
		nil,
		&testLogger{},
	)

	_, err := uc.Execute(context.Background(), dto.ExecuteUpgradeInput{SkipGovernance: true, HomeDir: "/tmp/devnet", Mode: types.ExecutionModeLocal})
	if err == nil || !strings.Contains(err.Error(), "no target binary specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteUpgradeUseCase_ExecuteWithGovPreExportErrors(t *testing.T) {
	t.Parallel()

	t.Run("pre-export failure", func(t *testing.T) {
		t.Parallel()
		uc := NewExecuteUpgradeUseCase(
			nil,
			nil,
			nil,
			&mockExportUseCase{executeFunc: func(ctx context.Context, input interface{}) (interface{}, error) {
				return nil, errors.New("export failed")
			}},
			&mockRPCClient{},
			&mockDevnetRepo{},
			nil,
			&testLogger{},
		)

		_, err := uc.Execute(context.Background(), dto.ExecuteUpgradeInput{SkipGovernance: false, WithExport: true})
		if err == nil || !strings.Contains(err.Error(), "pre-upgrade export failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid pre-export result type", func(t *testing.T) {
		t.Parallel()
		uc := NewExecuteUpgradeUseCase(
			nil,
			nil,
			nil,
			&mockExportUseCase{executeFunc: func(ctx context.Context, input interface{}) (interface{}, error) {
				return "invalid", nil
			}},
			&mockRPCClient{},
			&mockDevnetRepo{},
			nil,
			&testLogger{},
		)

		_, err := uc.Execute(context.Background(), dto.ExecuteUpgradeInput{SkipGovernance: false, WithExport: true})
		if err == nil || !strings.Contains(err.Error(), "invalid export result type") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestExecuteUpgradeUseCase_UpdateCurrentVersion(t *testing.T) {
	t.Parallel()

	metadata := &ports.DevnetMetadata{CurrentVersion: "v1"}
	repo := &mockDevnetRepo{
		loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
			return metadata, nil
		},
	}

	uc := &ExecuteUpgradeUseCase{devnetRepo: repo, logger: &testLogger{}}
	if err := uc.updateCurrentVersion(context.Background(), "/tmp/devnet", "v2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metadata.CurrentVersion != "v2" {
		t.Fatalf("version not updated: %s", metadata.CurrentVersion)
	}
}

func TestExecuteUpgradeUseCase_WaitForUpgradeHeight(t *testing.T) {
	t.Parallel()

	t.Run("success when already at target", func(t *testing.T) {
		t.Parallel()
		uc := &ExecuteUpgradeUseCase{
			rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 100, nil }},
			logger:    &testLogger{},
		}
		if err := uc.waitForUpgradeHeight(context.Background(), 100); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		t.Parallel()
		uc := &ExecuteUpgradeUseCase{
			rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 0, errors.New("rpc down") }},
			logger:    &testLogger{},
		}
		err := uc.waitForUpgradeHeight(context.Background(), 100)
		if err == nil || !strings.Contains(err.Error(), "failed to get block height") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("canceled while waiting", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		uc := &ExecuteUpgradeUseCase{
			rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				cancel()
				return 1, nil
			}},
			logger: &testLogger{},
		}
		err := uc.waitForUpgradeHeight(ctx, 10)
		if err == nil {
			t.Fatalf("expected cancellation error")
		}
	})
}

func TestExecuteUpgradeUseCase_WaitForChainHalt(t *testing.T) {
	t.Parallel()

	uc := &ExecuteUpgradeUseCase{
		rpcClient: &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 200, nil }},
		logger:    &testLogger{},
	}
	err := uc.waitForChainHalt(context.Background(), 100)
	if err == nil || !strings.Contains(err.Error(), "upgrade proposal may have failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = uc.waitForChainHalt(ctx, 100)
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
}

func TestExecuteUpgradeUseCase_VerifyChainResumedCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	uc := &ExecuteUpgradeUseCase{rpcClient: &mockRPCClient{}, logger: &testLogger{}}
	_, err := uc.verifyChainResumed(ctx, "/tmp/devnet")
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
}

func TestMonitorUseCase_Execute(t *testing.T) {
	t.Parallel()

	monitor := NewMonitorUseCase(&mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 55, nil }}, &testLogger{})
	ch, err := monitor.Execute(context.Background(), dto.MonitorInput{TargetHeight: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	progress := <-ch
	if !progress.IsComplete || progress.Stage != ports.StageCompleted {
		t.Fatalf("unexpected progress: %+v", progress)
	}
}

func TestProgressHelpers(t *testing.T) {
	t.Parallel()

	if bar := makeProgressBar(10, 50); bar != "=====>    " {
		t.Fatalf("unexpected bar: %q", bar)
	}
	if bar := makeProgressBar(5, -1); bar != ">    " {
		t.Fatalf("unexpected negative bar: %q", bar)
	}
	if bar := makeProgressBar(5, 200); bar != "=====" {
		t.Fatalf("unexpected overflow bar: %q", bar)
	}

	if got := formatDuration(65 * time.Second); got != "1m5s" {
		t.Fatalf("formatDuration = %q", got)
	}
	if got := formatDuration((2 * time.Hour) + (3 * time.Minute) + (4 * time.Second)); got != "2h3m4s" {
		t.Fatalf("formatDuration = %q", got)
	}
}

func TestExecuteUpgradeUseCase_ExecuteWithGov_ChainHaltErrorAfterProposalAndVote(t *testing.T) {
	teardown := startFakeEthRPCServer(t, 11)
	defer teardown()

	key := testValidatorKey(t)
	keyLoader := &mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
		return []ports.ValidatorKey{key}, nil
	}}

	proposeUC := NewProposeUseCase(&mockDevnetRepo{}, &mockRPCClient{}, keyLoader, &testLogger{})
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
	switchUC := NewSwitchBinaryUseCase(&mockDevnetRepo{}, &mockNodeRepo{}, &mockProcessExecutor{}, &mockBinaryCache{}, &testLogger{})

	rpcHeights := []int64{100, 120}
	execRPC := &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) {
		h := rpcHeights[0]
		rpcHeights = rpcHeights[1:]
		return h, nil
	}}

	uc := NewExecuteUpgradeUseCase(
		proposeUC,
		voteUC,
		switchUC,
		&mockExportUseCase{},
		execRPC,
		&mockDevnetRepo{},
		nil,
		&testLogger{},
	)

	_, err := uc.Execute(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:        "/tmp/devnet",
		UpgradeName:    "v2.0.0",
		UpgradeHeight:  100,
		VotingPeriod:   30 * time.Second,
		Mode:           types.ExecutionModeLocal,
		TargetBinary:   "/tmp/new-stabled",
		CacheRef:       "cache-ref",
		SkipGovernance: false,
	})
	if err == nil || !strings.Contains(err.Error(), "upgrade proposal may have failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteUpgradeUseCase_ExecuteWithGov_Success(t *testing.T) {
	teardown := startFakeEthRPCServer(t, 15)
	defer teardown()

	key := testValidatorKey(t)
	keyLoader := &mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
		return []ports.ValidatorKey{key}, nil
	}}

	proposeUC := NewProposeUseCase(&mockDevnetRepo{}, &mockRPCClient{}, keyLoader, &testLogger{})
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
	switchUC := NewSwitchBinaryUseCase(&mockDevnetRepo{}, &mockNodeRepo{}, &mockProcessExecutor{}, &mockBinaryCache{}, &testLogger{})

	rpcHeights := []int64{100, 100, 100, 100, 100, 101, 102}
	execRPC := &mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) {
		h := rpcHeights[0]
		rpcHeights = rpcHeights[1:]
		return h, nil
	}}

	uc := NewExecuteUpgradeUseCase(
		proposeUC,
		voteUC,
		switchUC,
		&mockExportUseCase{},
		execRPC,
		&mockDevnetRepo{},
		nil,
		&testLogger{},
	)

	out, err := uc.Execute(context.Background(), dto.ExecuteUpgradeInput{
		HomeDir:        "/tmp/devnet",
		UpgradeName:    "v2.0.0",
		UpgradeHeight:  100,
		VotingPeriod:   30 * time.Second,
		Mode:           types.ExecutionModeLocal,
		TargetBinary:   "/tmp/new-stabled",
		CacheRef:       "cache-ref",
		SkipGovernance: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success || out.ProposalID == 0 {
		t.Fatalf("unexpected output: %+v", out)
	}
}
