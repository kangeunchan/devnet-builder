// Package upgrade provides use cases for managing blockchain upgrades.
package upgrade

import (
	"context"
	"fmt"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/types"
)

// ResumableExecuteUpgradeUseCase wraps ExecuteUpgradeUseCase with state persistence.
// It saves state after each stage transition, enabling resume after interruptions.
type ResumableExecuteUpgradeUseCase struct {
	executeUC     *ExecuteUpgradeUseCase
	proposeUC     *ProposeUseCase
	voteUC        *VoteUseCase
	switchUC      *SwitchBinaryUseCase
	stateManager  ports.UpgradeStateManager
	transitioner  ports.UpgradeStateTransitioner
	stateDetector ports.UpgradeStateDetector
	rpcClient     ports.RPCClient
	exportUC      ports.ExportUseCase
	devnetRepo    ports.DevnetRepository
	logger        ports.Logger
	ops           *resumableUpgradeOps
}

type resumableUpgradeOps struct {
	executeProposal      func(context.Context, dto.ProposeInput) (*dto.ProposeOutput, error)
	executeVote          func(context.Context, dto.ExecuteUpgradeInput, *ports.UpgradeState) (*dto.VoteOutput, error)
	waitForUpgradeHeight func(context.Context, int64) error
	waitForChainHalt     func(context.Context, int64) error
	executeSwitchBinary  func(context.Context, dto.ExecuteUpgradeInput, *ports.UpgradeState) (*dto.SwitchBinaryOutput, error)
	verifyChainResumed   func(context.Context, string) (int64, error)
	executeExport        func(context.Context, dto.ExportInput) (interface{}, error)
	updateCurrentVersion func(context.Context, string, string) error
	deleteState          func(context.Context) error
	transitionAndSave    func(context.Context, *ports.UpgradeState, ports.ResumableStage, string) error
}

type resumableGovStageResult struct {
	err                   error
	preserveOutputOnError bool
}

type resumableGovStageHandler func(
	context.Context,
	dto.ExecuteUpgradeInput,
	*ports.UpgradeState,
	*dto.ExecuteUpgradeOutput,
) resumableGovStageResult

// NewResumableExecuteUpgradeUseCase creates a new ResumableExecuteUpgradeUseCase.
func NewResumableExecuteUpgradeUseCase(
	executeUC *ExecuteUpgradeUseCase,
	proposeUC *ProposeUseCase,
	voteUC *VoteUseCase,
	switchUC *SwitchBinaryUseCase,
	stateManager ports.UpgradeStateManager,
	transitioner ports.UpgradeStateTransitioner,
	stateDetector ports.UpgradeStateDetector,
	rpcClient ports.RPCClient,
	exportUC ports.ExportUseCase,
	devnetRepo ports.DevnetRepository,
	logger ports.Logger,
) *ResumableExecuteUpgradeUseCase {
	uc := &ResumableExecuteUpgradeUseCase{
		executeUC:     executeUC,
		proposeUC:     proposeUC,
		voteUC:        voteUC,
		switchUC:      switchUC,
		stateManager:  stateManager,
		transitioner:  transitioner,
		stateDetector: stateDetector,
		rpcClient:     rpcClient,
		exportUC:      exportUC,
		devnetRepo:    devnetRepo,
		logger:        logger,
	}
	uc.ops = uc.defaultOps()
	return uc
}

func (uc *ResumableExecuteUpgradeUseCase) defaultOps() *resumableUpgradeOps {
	return &resumableUpgradeOps{
		executeProposal: func(ctx context.Context, input dto.ProposeInput) (*dto.ProposeOutput, error) {
			return uc.proposeUC.Execute(ctx, input)
		},
		executeVote: func(ctx context.Context, input dto.ExecuteUpgradeInput, state *ports.UpgradeState) (*dto.VoteOutput, error) {
			return uc.executeVoting(ctx, input, state)
		},
		waitForUpgradeHeight: func(ctx context.Context, height int64) error {
			return uc.executeUC.waitForUpgradeHeight(ctx, height)
		},
		waitForChainHalt: func(ctx context.Context, height int64) error {
			return uc.executeUC.waitForChainHalt(ctx, height)
		},
		executeSwitchBinary: func(ctx context.Context, input dto.ExecuteUpgradeInput, state *ports.UpgradeState) (*dto.SwitchBinaryOutput, error) {
			return uc.executeSwitchBinary(ctx, input, state)
		},
		verifyChainResumed: func(ctx context.Context, homeDir string) (int64, error) {
			return uc.executeUC.verifyChainResumed(ctx, homeDir)
		},
		executeExport: func(ctx context.Context, input dto.ExportInput) (interface{}, error) {
			return uc.exportUC.Execute(ctx, input)
		},
		updateCurrentVersion: func(ctx context.Context, homeDir, version string) error {
			return uc.executeUC.updateCurrentVersion(ctx, homeDir, version)
		},
		deleteState: func(ctx context.Context) error {
			return uc.stateManager.DeleteState(ctx)
		},
		transitionAndSave: func(ctx context.Context, state *ports.UpgradeState, target ports.ResumableStage, reason string) error {
			return uc.transitionAndSave(ctx, state, target, reason)
		},
	}
}

func (uc *ResumableExecuteUpgradeUseCase) getOps() *resumableUpgradeOps {
	if uc.ops == nil {
		uc.ops = uc.defaultOps()
	}
	return uc.ops
}

// Execute performs the upgrade workflow with state persistence.
// If state exists and resume is requested, continues from the saved stage.
func (uc *ResumableExecuteUpgradeUseCase) Execute(ctx context.Context, input dto.ExecuteUpgradeInput, state *ports.UpgradeState) (*dto.ExecuteUpgradeOutput, error) {
	startTime := time.Now()

	// If no state provided, create new one
	if state == nil {
		mode := "local"
		if input.Mode == types.ExecutionModeDocker {
			mode = "docker"
		}
		state = ports.NewUpgradeState(input.UpgradeName, mode, input.SkipGovernance)

		// Initialize with input parameters
		state.TargetBinary = input.TargetBinary
		state.TargetImage = input.TargetImage
		state.TargetVersion = input.TargetVersion
		state.UpgradeHeight = input.UpgradeHeight

		// Save initial state
		if err := uc.saveState(ctx, state); err != nil {
			return nil, fmt.Errorf("failed to save initial state: %w", err)
		}
	}

	// Branch based on skip-gov mode
	if input.SkipGovernance {
		return uc.executeSkipGovResumable(ctx, input, state, startTime)
	}

	return uc.executeWithGovResumable(ctx, input, state, startTime)
}

// executeSkipGovResumable performs binary replacement without governance (resumable).
func (uc *ResumableExecuteUpgradeUseCase) executeSkipGovResumable(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	startTime time.Time,
) (*dto.ExecuteUpgradeOutput, error) {
	output := &dto.ExecuteUpgradeOutput{}

	// Resume from current stage
	switch state.Stage {
	case ports.ResumableStageInitialized:
		// Transition to SwitchingBinary
		if err := uc.transitionAndSave(ctx, state, ports.ResumableStageSwitchingBinary, "starting binary switch (skip-gov)"); err != nil {
			return nil, err
		}
		fallthrough

	case ports.ResumableStageSwitchingBinary:
		uc.logger.Info("Switching binary...")
		switchResult, err := uc.executeSwitchBinary(ctx, input, state)
		if err != nil {
			if saveErr := uc.transitionAndSave(ctx, state, ports.ResumableStageFailed, err.Error()); saveErr != nil {
				uc.logger.Warn("Failed to save failed state: %v", saveErr)
			}
			output.Error = err
			return output, err
		}
		output.NewBinary = switchResult.NewBinary

		// Transition to VerifyingResume
		if err := uc.transitionAndSave(ctx, state, ports.ResumableStageVerifyingResume, "binary switch complete"); err != nil {
			return nil, err
		}
		fallthrough

	case ports.ResumableStageVerifyingResume:
		uc.logger.Info("Verifying chain health...")
		postHeight, err := uc.executeUC.verifyChainResumed(ctx, input.HomeDir)
		if err != nil {
			if saveErr := uc.transitionAndSave(ctx, state, ports.ResumableStageFailed, err.Error()); saveErr != nil {
				uc.logger.Warn("Failed to save failed state: %v", saveErr)
			}
			output.Error = err
			return output, err
		}
		output.PostUpgradeHeight = postHeight

		// Transition to Completed
		if err := uc.transitionAndSave(ctx, state, ports.ResumableStageCompleted, "chain verified healthy"); err != nil {
			return nil, err
		}
		fallthrough

	case ports.ResumableStageCompleted:
		// Update metadata version
		if input.TargetVersion != "" {
			if err := uc.executeUC.updateCurrentVersion(ctx, input.HomeDir, input.TargetVersion); err != nil {
				uc.logger.Warn("Failed to update version in metadata: %v", err)
			}
		}

		// Delete state file on success
		if err := uc.stateManager.DeleteState(ctx); err != nil {
			uc.logger.Warn("Failed to delete state file: %v", err)
		}

		output.Success = true
		output.Duration = time.Since(startTime)
		uc.logger.Success("Binary replacement complete! Duration: %v", output.Duration)
		return output, nil

	default:
		return nil, fmt.Errorf("cannot resume from stage: %s", state.Stage)
	}
}

// executeWithGovResumable performs the full upgrade workflow with governance (resumable).
func (uc *ResumableExecuteUpgradeUseCase) executeWithGovResumable(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	startTime time.Time,
) (*dto.ExecuteUpgradeOutput, error) {
	output := &dto.ExecuteUpgradeOutput{}
	if err := uc.runPreUpgradeExport(ctx, input, state, output); err != nil {
		output.Error = err
		return output, err
	}

	handlers := uc.govResumableStageHandlers()
	for {
		if terminalOutput, terminalErr, done := uc.handleGovTerminalStage(ctx, input, state, startTime, output); done {
			return terminalOutput, terminalErr
		}

		handler, ok := handlers[state.Stage]
		if !ok {
			return nil, fmt.Errorf("cannot resume from stage: %s", state.Stage)
		}

		outcome := handler(ctx, input, state, output)
		if outcome.err != nil {
			if outcome.preserveOutputOnError {
				return output, outcome.err
			}
			return nil, outcome.err
		}
	}
}

func (uc *ResumableExecuteUpgradeUseCase) runPreUpgradeExport(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	output *dto.ExecuteUpgradeOutput,
) error {
	if state.Stage != ports.ResumableStageInitialized || !input.WithExport {
		return nil
	}
	ops := uc.getOps()

	uc.logger.Info("Pre-upgrade: Exporting state before upgrade...")
	exportInput := dto.ExportInput{
		HomeDir:   input.HomeDir,
		OutputDir: input.GenesisDir,
		Force:     false,
	}

	preExportResultRaw, err := ops.executeExport(ctx, exportInput)
	if err != nil {
		uc.logger.Error("Pre-upgrade export failed: %v", err)
		return fmt.Errorf("pre-upgrade export failed: %w", err)
	}
	if preExportResult, ok := preExportResultRaw.(*dto.ExportOutput); ok {
		output.PreGenesisPath = preExportResult.ExportPath
		uc.logger.Success("Pre-upgrade export complete: %s", preExportResult.ExportPath)
	}

	return nil
}

func (uc *ResumableExecuteUpgradeUseCase) govResumableStageHandlers() map[ports.ResumableStage]resumableGovStageHandler {
	return map[ports.ResumableStage]resumableGovStageHandler{
		ports.ResumableStageInitialized:       uc.handleGovStageInitialized,
		ports.ResumableStageProposalSubmitted: uc.handleGovStageProposalSubmitted,
		ports.ResumableStageVoting:            uc.handleGovStageVoting,
		ports.ResumableStageWaitingForHeight:  uc.handleGovStageWaitingForHeight,
		ports.ResumableStageChainHalted:       uc.handleGovStageChainHalted,
		ports.ResumableStageSwitchingBinary:   uc.handleGovStageSwitchingBinary,
		ports.ResumableStageVerifyingResume:   uc.handleGovStageVerifyingResume,
	}
}

func (uc *ResumableExecuteUpgradeUseCase) handleGovTerminalStage(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	startTime time.Time,
	output *dto.ExecuteUpgradeOutput,
) (*dto.ExecuteUpgradeOutput, error, bool) {
	ops := uc.getOps()

	switch state.Stage {
	case ports.ResumableStageCompleted:
		if input.TargetVersion != "" {
			if err := ops.updateCurrentVersion(ctx, input.HomeDir, input.TargetVersion); err != nil {
				uc.logger.Warn("Failed to update version in metadata: %v", err)
			}
		}

		if err := ops.deleteState(ctx); err != nil {
			uc.logger.Warn("Failed to delete state file: %v", err)
		}

		output.Success = true
		output.Duration = time.Since(startTime)
		uc.logger.Success("Upgrade complete! Duration: %v", output.Duration)
		return output, nil, true
	case ports.ResumableStageFailed:
		return nil, fmt.Errorf("upgrade previously failed: %s (use --force-restart to start fresh)", state.Error), true
	case ports.ResumableStageProposalRejected:
		return nil, fmt.Errorf("proposal was rejected (use --force-restart to start fresh)"), true
	default:
		return nil, nil, false
	}
}

func (uc *ResumableExecuteUpgradeUseCase) handleGovStageInitialized(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	output *dto.ExecuteUpgradeOutput,
) resumableGovStageResult {
	ops := uc.getOps()
	uc.logger.Info("Step 1/5: Submitting upgrade proposal...")

	proposeResult, err := ops.executeProposal(ctx, dto.ProposeInput{
		HomeDir:       input.HomeDir,
		UpgradeName:   input.UpgradeName,
		UpgradeHeight: input.UpgradeHeight,
		VotingPeriod:  input.VotingPeriod,
		HeightBuffer:  input.HeightBuffer,
	})
	if err != nil {
		return uc.failGovStage(ctx, state, output, err)
	}

	state.ProposalID = proposeResult.ProposalID
	state.UpgradeHeight = proposeResult.UpgradeHeight
	output.ProposalID = proposeResult.ProposalID
	output.UpgradeHeight = proposeResult.UpgradeHeight

	return uc.advanceGovStage(
		ctx,
		state,
		ports.ResumableStageProposalSubmitted,
		fmt.Sprintf("proposal %d submitted", proposeResult.ProposalID),
	)
}

func (uc *ResumableExecuteUpgradeUseCase) handleGovStageProposalSubmitted(
	ctx context.Context,
	_ dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	_ *dto.ExecuteUpgradeOutput,
) resumableGovStageResult {
	return uc.advanceGovStage(ctx, state, ports.ResumableStageVoting, "voting period started")
}

func (uc *ResumableExecuteUpgradeUseCase) handleGovStageVoting(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	output *dto.ExecuteUpgradeOutput,
) resumableGovStageResult {
	ops := uc.getOps()
	uc.logger.Info("Step 2/5: Voting from all validators...")
	output.ProposalID = state.ProposalID
	output.UpgradeHeight = state.UpgradeHeight

	voteResult, err := ops.executeVote(ctx, input, state)
	if err != nil {
		return uc.failGovStage(ctx, state, output, err)
	}

	if voteResult.VotesCast != voteResult.TotalVoters {
		return uc.failGovStage(ctx, state, output, fmt.Errorf("not all votes cast: %d/%d", voteResult.VotesCast, voteResult.TotalVoters))
	}

	return uc.advanceGovStage(ctx, state, ports.ResumableStageWaitingForHeight, "voting complete, proposal passed")
}

func (uc *ResumableExecuteUpgradeUseCase) handleGovStageWaitingForHeight(
	ctx context.Context,
	_ dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	output *dto.ExecuteUpgradeOutput,
) resumableGovStageResult {
	ops := uc.getOps()
	uc.logger.Info("Step 3/5: Waiting for upgrade height %d...", state.UpgradeHeight)
	output.ProposalID = state.ProposalID
	output.UpgradeHeight = state.UpgradeHeight

	if err := ops.waitForUpgradeHeight(ctx, state.UpgradeHeight); err != nil {
		return uc.failGovStage(ctx, state, output, err)
	}

	return uc.advanceGovStage(ctx, state, ports.ResumableStageChainHalted, "upgrade height reached")
}

func (uc *ResumableExecuteUpgradeUseCase) handleGovStageChainHalted(
	ctx context.Context,
	_ dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	output *dto.ExecuteUpgradeOutput,
) resumableGovStageResult {
	ops := uc.getOps()
	uc.logger.Info("Step 4/5: Waiting for chain to halt...")
	output.ProposalID = state.ProposalID
	output.UpgradeHeight = state.UpgradeHeight

	if err := ops.waitForChainHalt(ctx, state.UpgradeHeight); err != nil {
		return uc.failGovStage(ctx, state, output, err)
	}

	return uc.advanceGovStage(ctx, state, ports.ResumableStageSwitchingBinary, "chain halted at upgrade height")
}

func (uc *ResumableExecuteUpgradeUseCase) handleGovStageSwitchingBinary(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	output *dto.ExecuteUpgradeOutput,
) resumableGovStageResult {
	ops := uc.getOps()
	uc.logger.Info("Step 5/5: Switching binary...")
	output.ProposalID = state.ProposalID
	output.UpgradeHeight = state.UpgradeHeight

	switchResult, err := ops.executeSwitchBinary(ctx, input, state)
	if err != nil {
		return uc.failGovStage(ctx, state, output, err)
	}
	output.NewBinary = switchResult.NewBinary

	return uc.advanceGovStage(ctx, state, ports.ResumableStageVerifyingResume, "binary switch complete")
}

func (uc *ResumableExecuteUpgradeUseCase) handleGovStageVerifyingResume(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
	output *dto.ExecuteUpgradeOutput,
) resumableGovStageResult {
	ops := uc.getOps()
	output.ProposalID = state.ProposalID
	output.UpgradeHeight = state.UpgradeHeight

	postHeight, err := ops.verifyChainResumed(ctx, input.HomeDir)
	if err != nil {
		return uc.failGovStage(ctx, state, output, err)
	}
	output.PostUpgradeHeight = postHeight

	if input.WithExport {
		uc.logger.Info("Post-upgrade: Exporting state after upgrade...")
		exportInput := dto.ExportInput{
			HomeDir:   input.HomeDir,
			OutputDir: input.GenesisDir,
			Force:     false,
		}

		postExportResultRaw, err := ops.executeExport(ctx, exportInput)
		if err != nil {
			uc.logger.Warn("Post-upgrade export failed: %v", err)
		} else if postExportResult, ok := postExportResultRaw.(*dto.ExportOutput); ok {
			output.PostGenesisPath = postExportResult.ExportPath
			uc.logger.Success("Post-upgrade export complete: %s", postExportResult.ExportPath)
		}
	}

	return uc.advanceGovStage(ctx, state, ports.ResumableStageCompleted, "chain verified healthy")
}

func (uc *ResumableExecuteUpgradeUseCase) failGovStage(
	ctx context.Context,
	state *ports.UpgradeState,
	output *dto.ExecuteUpgradeOutput,
	err error,
) resumableGovStageResult {
	ops := uc.getOps()
	if saveErr := ops.transitionAndSave(ctx, state, ports.ResumableStageFailed, err.Error()); saveErr != nil {
		uc.logger.Warn("Failed to save failed state: %v", saveErr)
	}
	output.Error = err
	return resumableGovStageResult{err: err, preserveOutputOnError: true}
}

func (uc *ResumableExecuteUpgradeUseCase) advanceGovStage(
	ctx context.Context,
	state *ports.UpgradeState,
	target ports.ResumableStage,
	reason string,
) resumableGovStageResult {
	ops := uc.getOps()
	if err := ops.transitionAndSave(ctx, state, target, reason); err != nil {
		return resumableGovStageResult{err: err}
	}
	return resumableGovStageResult{}
}

// executeSwitchBinary handles binary switching with per-node tracking.
func (uc *ResumableExecuteUpgradeUseCase) executeSwitchBinary(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
) (*dto.SwitchBinaryOutput, error) {
	result, err := uc.switchUC.Execute(ctx, dto.SwitchBinaryInput{
		HomeDir:       input.HomeDir,
		TargetBinary:  input.TargetBinary,
		TargetImage:   input.TargetImage,
		TargetVersion: input.TargetVersion,
		CachePath:     input.CachePath,
		CommitHash:    input.CommitHash,
		CacheRef:      input.CacheRef,
		Mode:          input.Mode,
		UpgradeHeight: state.UpgradeHeight,
	})
	if err != nil {
		return nil, err
	}

	// Update node switch tracking in state
	now := time.Now()
	for i := 0; i < result.NodesRestarted; i++ {
		nodeName := fmt.Sprintf("node%d", i)
		// Find or add node switch state
		found := false
		for j := range state.NodeSwitches {
			if state.NodeSwitches[j].NodeName == nodeName {
				state.NodeSwitches[j].Switched = true
				state.NodeSwitches[j].Stopped = true
				state.NodeSwitches[j].Started = true
				state.NodeSwitches[j].OldBinary = result.OldBinary
				state.NodeSwitches[j].NewBinary = result.NewBinary
				state.NodeSwitches[j].Timestamp = &now
				found = true
				break
			}
		}
		if !found {
			state.NodeSwitches = append(state.NodeSwitches, ports.NodeSwitchState{
				NodeName:  nodeName,
				Switched:  true,
				Stopped:   true,
				Started:   true,
				OldBinary: result.OldBinary,
				NewBinary: result.NewBinary,
				Timestamp: &now,
			})
		}
	}

	// Save updated state
	if err := uc.saveState(ctx, state); err != nil {
		uc.logger.Warn("Failed to save node switch state: %v", err)
	}

	return result, nil
}

// executeVoting handles voting with per-validator tracking.
func (uc *ResumableExecuteUpgradeUseCase) executeVoting(
	ctx context.Context,
	input dto.ExecuteUpgradeInput,
	state *ports.UpgradeState,
) (*dto.VoteOutput, error) {
	result, err := uc.voteUC.Execute(ctx, dto.VoteInput{
		HomeDir:    input.HomeDir,
		ProposalID: state.ProposalID,
		VoteOption: "yes",
		FromAll:    true,
	})
	if err != nil {
		return nil, err
	}

	// Update validator vote tracking in state
	now := time.Now()
	for i, txHash := range result.TxHashes {
		// Find or add validator vote state
		if i >= len(state.ValidatorVotes) {
			state.ValidatorVotes = append(state.ValidatorVotes, ports.ValidatorVoteState{
				Address:   fmt.Sprintf("validator-%d", i),
				Voted:     true,
				TxHash:    txHash,
				Timestamp: &now,
			})
		} else {
			state.ValidatorVotes[i].Voted = true
			state.ValidatorVotes[i].TxHash = txHash
			state.ValidatorVotes[i].Timestamp = &now
		}
	}

	// Save updated state
	if err := uc.saveState(ctx, state); err != nil {
		uc.logger.Warn("Failed to save vote state: %v", err)
	}

	return result, nil
}

// transitionAndSave transitions the state and saves it to disk.
func (uc *ResumableExecuteUpgradeUseCase) transitionAndSave(
	ctx context.Context,
	state *ports.UpgradeState,
	target ports.ResumableStage,
	reason string,
) error {
	if err := uc.transitioner.TransitionTo(state, target, reason); err != nil {
		return fmt.Errorf("invalid state transition to %s: %w", target, err)
	}

	if err := uc.saveState(ctx, state); err != nil {
		return fmt.Errorf("failed to save state after transition to %s: %w", target, err)
	}

	uc.logger.Debug("State transitioned: %s -> %s (%s)", state.StageHistory[len(state.StageHistory)-1].From, target, reason)
	return nil
}

// saveState saves the current state to disk.
func (uc *ResumableExecuteUpgradeUseCase) saveState(ctx context.Context, state *ports.UpgradeState) error {
	state.UpdatedAt = time.Now()
	return uc.stateManager.SaveState(ctx, state)
}

// GetCurrentState returns the current upgrade state.
func (uc *ResumableExecuteUpgradeUseCase) GetCurrentState(ctx context.Context) (*ports.UpgradeState, error) {
	return uc.stateManager.LoadState(ctx)
}

// ClearState clears the upgrade state file.
func (uc *ResumableExecuteUpgradeUseCase) ClearState(ctx context.Context) error {
	return uc.stateManager.DeleteState(ctx)
}
