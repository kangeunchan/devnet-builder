package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestProposeUseCase_Execute_Errors(t *testing.T) {
	t.Parallel()

	t.Run("load devnet fails", func(t *testing.T) {
		t.Parallel()
		uc := NewProposeUseCase(
			&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
				return nil, errors.New("load fail")
			}},
			&mockRPCClient{},
			&mockValidatorKeyLoader{},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.ProposeInput{HomeDir: "/tmp/devnet", UpgradeHeight: 10})
		if err == nil || !strings.Contains(err.Error(), "failed to load devnet") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("devnet not running", func(t *testing.T) {
		t.Parallel()
		uc := NewProposeUseCase(
			&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
				return &ports.DevnetMetadata{Status: ports.StateStopped}, nil
			}},
			&mockRPCClient{},
			&mockValidatorKeyLoader{},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.ProposeInput{HomeDir: "/tmp/devnet", UpgradeHeight: 10})
		if err == nil || err.Error() != "devnet is not running" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("calculate height fails", func(t *testing.T) {
		t.Parallel()
		uc := NewProposeUseCase(
			&mockDevnetRepo{},
			&mockRPCClient{getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 0, errors.New("height fail") }},
			&mockValidatorKeyLoader{},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.ProposeInput{HomeDir: "/tmp/devnet", UpgradeHeight: 0})
		if err == nil || !strings.Contains(err.Error(), "failed to calculate upgrade height") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validator key load fails", func(t *testing.T) {
		t.Parallel()
		uc := NewProposeUseCase(
			&mockDevnetRepo{},
			&mockRPCClient{},
			&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
				return nil, errors.New("key load fail")
			}},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.ProposeInput{HomeDir: "/tmp/devnet", UpgradeHeight: 10})
		if err == nil || !strings.Contains(err.Error(), "failed to load validator keys") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestProposeUseCase_CalculateUpgradeHeight(t *testing.T) {
	t.Parallel()

	rpc := &mockRPCClient{
		getBlockHeightFunc: func(ctx context.Context) (int64, error) { return 100, nil },
		getGovParamsFunc:   func(ctx context.Context) (*ports.GovParams, error) { return nil, errors.New("gov fail") },
		getBlockTimeFunc:   func(ctx context.Context, sampleSize int) (time.Duration, error) { return 0, errors.New("bt fail") },
	}
	uc := NewProposeUseCase(&mockDevnetRepo{}, rpc, &mockValidatorKeyLoader{}, &testLogger{})

	height, err := uc.calculateUpgradeHeight(context.Background(), dto.ProposeInput{VotingPeriod: 30 * time.Second, HeightBuffer: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if height <= 100 {
		t.Fatalf("expected calculated height > 100, got %d", height)
	}
}

func TestProposeUseCase_CalculateHeightBuffer(t *testing.T) {
	t.Parallel()

	uc := NewProposeUseCase(&mockDevnetRepo{}, &mockRPCClient{}, &mockValidatorKeyLoader{}, &testLogger{})

	if got := uc.calculateHeightBuffer(context.Background(), 1, time.Second); got != 40 {
		t.Fatalf("expected default buffer 40, got %d", got)
	}
	if got := uc.calculateHeightBuffer(context.Background(), 10, 20*time.Second); got != 10 {
		t.Fatalf("expected minimum buffer 10, got %d", got)
	}
	if got := uc.calculateHeightBuffer(context.Background(), 10, 10*time.Millisecond); got != 200 {
		t.Fatalf("expected max cap 200, got %d", got)
	}
}

func TestProposalHelpers(t *testing.T) {
	t.Parallel()

	proposalJSON := buildProposalJSON("v2.0.0", 1234, "info")
	if !strings.Contains(proposalJSON, "v2.0.0") || !strings.Contains(proposalJSON, "1234") {
		t.Fatalf("proposal JSON missing expected fields: %s", proposalJSON)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(proposalJSON), &parsed); err != nil {
		t.Fatalf("invalid proposal JSON: %v", err)
	}

	callData, err := buildSubmitProposalCallData("0x0000000000000000000000000000000000000001", proposalJSON, "astable", "100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(callData) < 4 {
		t.Fatalf("callData too short")
	}
	wantMethodID := crypto.Keccak256([]byte("submitProposal(address,bytes,(string,uint256)[])"))[:4]
	if got := hex.EncodeToString(callData[:4]); got != hex.EncodeToString(wantMethodID) {
		t.Fatalf("methodID mismatch: got %s", got)
	}
}

func TestParseProposalIDFromLogs(t *testing.T) {
	t.Parallel()

	eventSig := crypto.Keccak256Hash([]byte("SubmitProposal(address,uint64)"))

	id := uint64(42)
	idBig := new(big.Int).SetUint64(id)
	data := common.LeftPadBytes(idBig.Bytes(), 32)
	logs := []*ethtypes.Log{{Topics: []common.Hash{eventSig}, Data: data}}

	parsedID, err := parseProposalIDFromLogs(logs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsedID != id {
		t.Fatalf("id = %d, want %d", parsedID, id)
	}

	fallbackLogs := []*ethtypes.Log{{Topics: []common.Hash{common.HexToHash("0x01"), common.BigToHash(big.NewInt(77))}}}
	parsedID, err = parseProposalIDFromLogs(fallbackLogs)
	if err != nil {
		t.Fatalf("unexpected fallback error: %v", err)
	}
	if parsedID != 77 {
		t.Fatalf("fallback id = %d", parsedID)
	}

	_, err = parseProposalIDFromLogs([]*ethtypes.Log{{Topics: []common.Hash{common.HexToHash("0x01")}, Data: []byte("short")}})
	if err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestVoteUseCase_Execute_Errors(t *testing.T) {
	t.Parallel()

	t.Run("devnet load fails", func(t *testing.T) {
		t.Parallel()
		uc := NewVoteUseCase(
			&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
				return nil, errors.New("load fail")
			}},
			&mockRPCClient{},
			&mockValidatorKeyLoader{},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.VoteInput{HomeDir: "/tmp/devnet", ProposalID: 1, VoteOption: "yes"})
		if err == nil || !strings.Contains(err.Error(), "failed to load devnet") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("proposal not in voting", func(t *testing.T) {
		t.Parallel()
		uc := NewVoteUseCase(
			&mockDevnetRepo{},
			&mockRPCClient{getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
				return &ports.Proposal{Status: ports.ProposalStatusPassed}, nil
			}},
			&mockValidatorKeyLoader{},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.VoteInput{HomeDir: "/tmp/devnet", ProposalID: 1, VoteOption: "yes"})
		if err == nil || !strings.Contains(err.Error(), "proposal is not in voting period") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validator key load fails", func(t *testing.T) {
		t.Parallel()
		uc := NewVoteUseCase(
			&mockDevnetRepo{},
			&mockRPCClient{},
			&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
				return nil, errors.New("key fail")
			}},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.VoteInput{HomeDir: "/tmp/devnet", ProposalID: 1, VoteOption: "yes"})
		if err == nil || !strings.Contains(err.Error(), "failed to load validator keys") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid vote option", func(t *testing.T) {
		t.Parallel()
		uc := NewVoteUseCase(
			&mockDevnetRepo{},
			&mockRPCClient{},
			&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
				return []ports.ValidatorKey{}, nil
			}},
			&testLogger{},
		)
		_, err := uc.Execute(context.Background(), dto.VoteInput{HomeDir: "/tmp/devnet", ProposalID: 1, VoteOption: "invalid"})
		if err == nil || !strings.Contains(err.Error(), "invalid vote option") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVoteHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  int
	}{
		{input: "yes", want: VoteOptionYes},
		{input: "no", want: VoteOptionNo},
		{input: "abstain", want: VoteOptionAbstain},
		{input: "no_with_veto", want: VoteOptionNoWithVeto},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseVoteOption(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("option = %d, want %d", got, tt.want)
			}
		})
	}

	if _, err := ParseVoteOption("bad"); err == nil {
		t.Fatalf("expected invalid option error")
	}

	data := buildVoteCallData("0x0000000000000000000000000000000000000001", 7, VoteOptionYes, "")
	if len(data) < 4 {
		t.Fatalf("call data too short")
	}
	method := crypto.Keccak256([]byte("vote(address,uint64,uint8,string)"))[:4]
	if hex.EncodeToString(data[:4]) != hex.EncodeToString(method) {
		t.Fatalf("method id mismatch")
	}
}

func TestBuildSubmitProposalCallData_IsDeterministic(t *testing.T) {
	t.Parallel()

	proposal := buildProposalJSON("v3", 999, "deterministic")
	a, err := buildSubmitProposalCallData("0x0000000000000000000000000000000000000001", proposal, "astable", "100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := buildSubmitProposalCallData("0x0000000000000000000000000000000000000001", proposal, "astable", "100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shaA := sha256.Sum256(a)
	shaB := sha256.Sum256(b)
	if shaA != shaB {
		t.Fatalf("expected deterministic call data")
	}
}
