package upgrade

import (
	"context"
	"errors"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

func TestStateDetector_DetectProposalStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		proposalID uint64
		proposal   *ports.Proposal
		err        error
		want       string
		wantErr    bool
	}{
		{name: "invalid id", proposalID: 0, want: "unknown", wantErr: true},
		{name: "voting", proposalID: 1, proposal: &ports.Proposal{Status: ports.ProposalStatusVoting}, want: "voting"},
		{name: "passed", proposalID: 1, proposal: &ports.Proposal{Status: ports.ProposalStatusPassed}, want: "passed"},
		{name: "rejected", proposalID: 1, proposal: &ports.Proposal{Status: ports.ProposalStatusRejected}, want: "rejected"},
		{name: "failed", proposalID: 1, proposal: &ports.Proposal{Status: ports.ProposalStatusFailed}, want: "failed"},
		{name: "pending", proposalID: 1, proposal: &ports.Proposal{Status: ports.ProposalStatusPending}, want: "pending"},
		{name: "unknown status", proposalID: 1, proposal: &ports.Proposal{Status: "UNKNOWN"}, want: "unknown"},
		{name: "rpc error", proposalID: 1, err: errors.New("rpc"), want: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := NewStateDetector(&mockRPCClient{
				getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
					if tt.err != nil {
						return nil, tt.err
					}
					return tt.proposal, nil
				},
			})

			got, err := d.DetectProposalStatus(context.Background(), tt.proposalID)
			if got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestStateDetector_DetectChainStatus(t *testing.T) {
	t.Parallel()

	t.Run("unreachable when chain not running", func(t *testing.T) {
		t.Parallel()
		d := NewStateDetector(&mockRPCClient{isChainRunningFunc: func(ctx context.Context) bool { return false }})
		status, err := d.DetectChainStatus(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "unreachable" {
			t.Fatalf("status = %q, want unreachable", status)
		}
	})

	t.Run("unreachable when first height query fails", func(t *testing.T) {
		t.Parallel()
		d := NewStateDetector(&mockRPCClient{
			isChainRunningFunc: func(ctx context.Context) bool { return true },
			getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 0, errors.New("boom") },
		})
		status, err := d.DetectChainStatus(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "unreachable" {
			t.Fatalf("status = %q, want unreachable", status)
		}
	})

	t.Run("halted on cancelled context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		d := NewStateDetector(&mockRPCClient{
			isChainRunningFunc: func(ctx context.Context) bool { return true },
			getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 10, nil },
		})
		status, err := d.DetectChainStatus(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "halted" {
			t.Fatalf("status = %q, want halted", status)
		}
	})

	t.Run("running when height advances", func(t *testing.T) {
		heights := []int64{10, 12}
		d := NewStateDetector(&mockRPCClient{
			isChainRunningFunc: func(ctx context.Context) bool { return true },
			getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			},
		})
		status, err := d.DetectChainStatus(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "running" {
			t.Fatalf("status = %q, want running", status)
		}
	})
}

func TestStateDetector_DetectValidatorVotes(t *testing.T) {
	t.Parallel()

	d := NewStateDetector(&mockRPCClient{})
	if _, err := d.DetectValidatorVotes(context.Background(), 0); err == nil {
		t.Fatalf("expected error for invalid proposal ID")
	}
	votes, err := d.DetectValidatorVotes(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(votes) != 0 {
		t.Fatalf("expected empty votes, got %d", len(votes))
	}
}

func TestStateDetector_DetectCurrentStage(t *testing.T) {
	t.Parallel()

	d := NewStateDetector(&mockRPCClient{})
	if _, err := d.DetectCurrentStage(context.Background(), nil); err == nil {
		t.Fatalf("expected nil state error")
	}
}

func TestStateDetector_DetectSkipGovStage(t *testing.T) {
	t.Parallel()

	t.Run("initialized when no switches", func(t *testing.T) {
		t.Parallel()
		d := NewStateDetector(&mockRPCClient{})
		state := &ports.UpgradeState{SkipGovernance: true, Stage: ports.ResumableStageInitialized}
		stage, err := d.DetectCurrentStage(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stage != ports.ResumableStageInitialized {
			t.Fatalf("stage = %s", stage)
		}
	})

	t.Run("switching when partially switched", func(t *testing.T) {
		t.Parallel()
		d := NewStateDetector(&mockRPCClient{})
		state := &ports.UpgradeState{
			SkipGovernance: true,
			Stage:          ports.ResumableStageSwitchingBinary,
			NodeSwitches: []ports.NodeSwitchState{
				{NodeName: "node0", Switched: true},
				{NodeName: "node1", Switched: false},
			},
		}
		stage, err := d.DetectCurrentStage(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stage != ports.ResumableStageSwitchingBinary {
			t.Fatalf("stage = %s", stage)
		}
	})

	t.Run("verifying when all switched and chain running", func(t *testing.T) {
		heights := []int64{10, 11}
		d := NewStateDetector(&mockRPCClient{
			isChainRunningFunc: func(ctx context.Context) bool { return true },
			getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			},
		})
		state := &ports.UpgradeState{
			SkipGovernance: true,
			Stage:          ports.ResumableStageSwitchingBinary,
			NodeSwitches: []ports.NodeSwitchState{
				{NodeName: "node0", Switched: true},
				{NodeName: "node1", Switched: true},
			},
		}
		stage, err := d.DetectCurrentStage(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stage != ports.ResumableStageVerifyingResume {
			t.Fatalf("stage = %s", stage)
		}
	})
}

func TestStateDetector_DetectGovPathStages(t *testing.T) {
	t.Parallel()

	t.Run("initialized when proposal id missing", func(t *testing.T) {
		t.Parallel()
		d := NewStateDetector(&mockRPCClient{})
		state := &ports.UpgradeState{Stage: ports.ResumableStageInitialized, ProposalID: 0}
		stage, err := d.DetectCurrentStage(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stage != ports.ResumableStageInitialized {
			t.Fatalf("stage = %s", stage)
		}
	})

	t.Run("status mapping", func(t *testing.T) {
		tests := []struct {
			name   string
			status ports.ProposalStatus
			want   ports.ResumableStage
		}{
			{name: "pending", status: ports.ProposalStatusPending, want: ports.ResumableStageProposalSubmitted},
			{name: "voting", status: ports.ProposalStatusVoting, want: ports.ResumableStageVoting},
			{name: "rejected", status: ports.ProposalStatusRejected, want: ports.ResumableStageProposalRejected},
			{name: "failed", status: ports.ProposalStatusFailed, want: ports.ResumableStageFailed},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				d := NewStateDetector(&mockRPCClient{
					getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
						return &ports.Proposal{Status: tt.status}, nil
					},
				})
				state := &ports.UpgradeState{Stage: ports.ResumableStageProposalSubmitted, ProposalID: 10}
				stage, err := d.DetectCurrentStage(context.Background(), state)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if stage != tt.want {
					t.Fatalf("stage = %s, want %s", stage, tt.want)
				}
			})
		}
	})

	t.Run("passed stage with zero upgrade height", func(t *testing.T) {
		t.Parallel()
		d := NewStateDetector(&mockRPCClient{
			getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
				return &ports.Proposal{Status: ports.ProposalStatusPassed}, nil
			},
		})
		state := &ports.UpgradeState{Stage: ports.ResumableStageVoting, ProposalID: 1, UpgradeHeight: 0}
		stage, err := d.DetectCurrentStage(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stage != ports.ResumableStageWaitingForHeight {
			t.Fatalf("stage = %s", stage)
		}
	})

	t.Run("passed stage halted chain", func(t *testing.T) {
		t.Parallel()
		d := NewStateDetector(&mockRPCClient{
			getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
				return &ports.Proposal{Status: ports.ProposalStatusPassed}, nil
			},
			isChainRunningFunc: func(ctx context.Context) bool { return false },
		})
		state := &ports.UpgradeState{Stage: ports.ResumableStageWaitingForHeight, ProposalID: 1, UpgradeHeight: 100}
		stage, err := d.DetectCurrentStage(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stage != ports.ResumableStageChainHalted {
			t.Fatalf("stage = %s", stage)
		}
	})

	t.Run("passed stage running before target height", func(t *testing.T) {
		heights := []int64{90, 91, 91}
		d := NewStateDetector(&mockRPCClient{
			getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
				return &ports.Proposal{Status: ports.ProposalStatusPassed}, nil
			},
			isChainRunningFunc: func(ctx context.Context) bool { return true },
			getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			},
		})
		state := &ports.UpgradeState{Stage: ports.ResumableStageWaitingForHeight, ProposalID: 1, UpgradeHeight: 100}
		stage, err := d.DetectCurrentStage(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stage != ports.ResumableStageWaitingForHeight {
			t.Fatalf("stage = %s", stage)
		}
	})

	t.Run("passed stage running with all switches", func(t *testing.T) {
		heights := []int64{120, 121, 121}
		d := NewStateDetector(&mockRPCClient{
			getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
				return &ports.Proposal{Status: ports.ProposalStatusPassed}, nil
			},
			isChainRunningFunc: func(ctx context.Context) bool { return true },
			getBlockHeightFunc: func(ctx context.Context) (int64, error) {
				h := heights[0]
				heights = heights[1:]
				return h, nil
			},
		})
		state := &ports.UpgradeState{
			Stage:         ports.ResumableStageSwitchingBinary,
			ProposalID:    1,
			UpgradeHeight: 100,
			NodeSwitches: []ports.NodeSwitchState{
				{NodeName: "node0", Switched: true},
				{NodeName: "node1", Switched: true},
			},
		}
		stage, err := d.DetectCurrentStage(context.Background(), state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stage != ports.ResumableStageVerifyingResume {
			t.Fatalf("stage = %s", stage)
		}
	})
}
