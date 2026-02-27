package upgrade

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/types"
)

type testLogger struct{}

func (l *testLogger) Info(format string, args ...interface{})    {}
func (l *testLogger) Warn(format string, args ...interface{})    {}
func (l *testLogger) Error(format string, args ...interface{})   {}
func (l *testLogger) Debug(format string, args ...interface{})   {}
func (l *testLogger) Success(format string, args ...interface{}) {}
func (l *testLogger) Print(format string, args ...interface{})   {}
func (l *testLogger) Println(format string, args ...interface{}) {}
func (l *testLogger) SetVerbose(verbose bool)                    {}
func (l *testLogger) IsVerbose() bool                            { return false }
func (l *testLogger) Writer() io.Writer                          { return os.Stdout }
func (l *testLogger) ErrWriter() io.Writer                       { return os.Stderr }

type mockRPCClient struct {
	getBlockHeightFunc func(ctx context.Context) (int64, error)
	getBlockTimeFunc   func(ctx context.Context, sampleSize int) (time.Duration, error)
	isChainRunningFunc func(ctx context.Context) bool
	waitForBlockFunc   func(ctx context.Context, height int64) error
	getProposalFunc    func(ctx context.Context, id uint64) (*ports.Proposal, error)
	getUpgradePlanFunc func(ctx context.Context) (*ports.UpgradePlan, error)
	getAppVersionFunc  func(ctx context.Context) (string, error)
	getGovParamsFunc   func(ctx context.Context) (*ports.GovParams, error)
}

func (m *mockRPCClient) GetBlockHeight(ctx context.Context) (int64, error) {
	if m.getBlockHeightFunc != nil {
		return m.getBlockHeightFunc(ctx)
	}
	return 0, nil
}

func (m *mockRPCClient) GetBlockTime(ctx context.Context, sampleSize int) (time.Duration, error) {
	if m.getBlockTimeFunc != nil {
		return m.getBlockTimeFunc(ctx, sampleSize)
	}
	return time.Second, nil
}

func (m *mockRPCClient) IsChainRunning(ctx context.Context) bool {
	if m.isChainRunningFunc != nil {
		return m.isChainRunningFunc(ctx)
	}
	return true
}

func (m *mockRPCClient) WaitForBlock(ctx context.Context, height int64) error {
	if m.waitForBlockFunc != nil {
		return m.waitForBlockFunc(ctx, height)
	}
	return nil
}

func (m *mockRPCClient) GetProposal(ctx context.Context, id uint64) (*ports.Proposal, error) {
	if m.getProposalFunc != nil {
		return m.getProposalFunc(ctx, id)
	}
	return &ports.Proposal{Status: ports.ProposalStatusVoting}, nil
}

func (m *mockRPCClient) GetUpgradePlan(ctx context.Context) (*ports.UpgradePlan, error) {
	if m.getUpgradePlanFunc != nil {
		return m.getUpgradePlanFunc(ctx)
	}
	return nil, nil
}

func (m *mockRPCClient) GetAppVersion(ctx context.Context) (string, error) {
	if m.getAppVersionFunc != nil {
		return m.getAppVersionFunc(ctx)
	}
	return "", nil
}

func (m *mockRPCClient) GetGovParams(ctx context.Context) (*ports.GovParams, error) {
	if m.getGovParamsFunc != nil {
		return m.getGovParamsFunc(ctx)
	}
	return &ports.GovParams{ExpeditedVotingPeriod: 60 * time.Second}, nil
}

type mockDevnetRepo struct {
	loadFunc   func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error)
	saveFunc   func(ctx context.Context, metadata *ports.DevnetMetadata) error
	deleteFunc func(ctx context.Context, homeDir string) error
	existsFunc func(homeDir string) bool
}

func (m *mockDevnetRepo) Save(ctx context.Context, metadata *ports.DevnetMetadata) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, metadata)
	}
	return nil
}

func (m *mockDevnetRepo) Load(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ctx, homeDir)
	}
	return &ports.DevnetMetadata{
		HomeDir:        homeDir,
		Status:         ports.StateRunning,
		ExecutionMode:  types.ExecutionModeLocal,
		CurrentVersion: "v1.0.0",
		BinaryName:     "stabled",
		NumValidators:  1,
	}, nil
}

func (m *mockDevnetRepo) Delete(ctx context.Context, homeDir string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, homeDir)
	}
	return nil
}

func (m *mockDevnetRepo) Exists(homeDir string) bool {
	if m.existsFunc != nil {
		return m.existsFunc(homeDir)
	}
	return true
}

type mockNodeRepo struct {
	loadAllFunc func(ctx context.Context, homeDir string) ([]*ports.NodeMetadata, error)
	loadFunc    func(ctx context.Context, homeDir string, index int) (*ports.NodeMetadata, error)
	saveFunc    func(ctx context.Context, node *ports.NodeMetadata) error
	deleteFunc  func(ctx context.Context, homeDir string, index int) error
}

func (m *mockNodeRepo) Save(ctx context.Context, node *ports.NodeMetadata) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, node)
	}
	return nil
}

func (m *mockNodeRepo) Load(ctx context.Context, homeDir string, index int) (*ports.NodeMetadata, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ctx, homeDir, index)
	}
	return &ports.NodeMetadata{Index: index, Name: "node"}, nil
}

func (m *mockNodeRepo) LoadAll(ctx context.Context, homeDir string) ([]*ports.NodeMetadata, error) {
	if m.loadAllFunc != nil {
		return m.loadAllFunc(ctx, homeDir)
	}
	return []*ports.NodeMetadata{}, nil
}

func (m *mockNodeRepo) Delete(ctx context.Context, homeDir string, index int) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, homeDir, index)
	}
	return nil
}

type mockProcessHandle struct {
	pid int
}

func (h *mockProcessHandle) PID() int        { return h.pid }
func (h *mockProcessHandle) IsRunning() bool { return true }
func (h *mockProcessHandle) Wait() error     { return nil }
func (h *mockProcessHandle) Kill() error     { return nil }

type mockProcessExecutor struct {
	startFunc     func(ctx context.Context, cmd ports.Command) (ports.ProcessHandle, error)
	stopFunc      func(ctx context.Context, handle ports.ProcessHandle, timeout time.Duration) error
	killFunc      func(handle ports.ProcessHandle) error
	isRunningFunc func(handle ports.ProcessHandle) bool
	logsFunc      func(handle ports.ProcessHandle, lines int) ([]string, error)
}

func (m *mockProcessExecutor) Start(ctx context.Context, cmd ports.Command) (ports.ProcessHandle, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx, cmd)
	}
	return &mockProcessHandle{pid: 1000}, nil
}

func (m *mockProcessExecutor) Stop(ctx context.Context, handle ports.ProcessHandle, timeout time.Duration) error {
	if m.stopFunc != nil {
		return m.stopFunc(ctx, handle, timeout)
	}
	return nil
}

func (m *mockProcessExecutor) Kill(handle ports.ProcessHandle) error {
	if m.killFunc != nil {
		return m.killFunc(handle)
	}
	return nil
}

func (m *mockProcessExecutor) IsRunning(handle ports.ProcessHandle) bool {
	if m.isRunningFunc != nil {
		return m.isRunningFunc(handle)
	}
	return true
}

func (m *mockProcessExecutor) Logs(handle ports.ProcessHandle, lines int) ([]string, error) {
	if m.logsFunc != nil {
		return m.logsFunc(handle, lines)
	}
	return []string{}, nil
}

type mockBinaryCache struct {
	storeFunc        func(ctx context.Context, ref string, binaryPath string) (string, error)
	getFunc          func(ref string) (string, bool)
	hasFunc          func(ref string) bool
	listFunc         func() []string
	listDetailedFunc func() []ports.CachedBinaryInfo
	statsFunc        func() ports.CacheStats
	removeFunc       func(ref string) error
	cleanFunc        func() error
	setActiveFunc    func(ref string) error
	getActiveFunc    func() (string, error)
	cacheDirFunc     func() string
	symlinkPathFunc  func() string
	symlinkInfoFunc  func() (*ports.SymlinkInfo, error)
}

func (m *mockBinaryCache) Store(ctx context.Context, ref string, binaryPath string) (string, error) {
	if m.storeFunc != nil {
		return m.storeFunc(ctx, ref, binaryPath)
	}
	return binaryPath, nil
}

func (m *mockBinaryCache) Get(ref string) (string, bool) {
	if m.getFunc != nil {
		return m.getFunc(ref)
	}
	return "", false
}

func (m *mockBinaryCache) Has(ref string) bool {
	if m.hasFunc != nil {
		return m.hasFunc(ref)
	}
	return false
}

func (m *mockBinaryCache) List() []string {
	if m.listFunc != nil {
		return m.listFunc()
	}
	return []string{}
}

func (m *mockBinaryCache) ListDetailed() []ports.CachedBinaryInfo {
	if m.listDetailedFunc != nil {
		return m.listDetailedFunc()
	}
	return []ports.CachedBinaryInfo{}
}

func (m *mockBinaryCache) Stats() ports.CacheStats {
	if m.statsFunc != nil {
		return m.statsFunc()
	}
	return ports.CacheStats{}
}

func (m *mockBinaryCache) Remove(ref string) error {
	if m.removeFunc != nil {
		return m.removeFunc(ref)
	}
	return nil
}

func (m *mockBinaryCache) Clean() error {
	if m.cleanFunc != nil {
		return m.cleanFunc()
	}
	return nil
}

func (m *mockBinaryCache) SetActive(ref string) error {
	if m.setActiveFunc != nil {
		return m.setActiveFunc(ref)
	}
	return nil
}

func (m *mockBinaryCache) GetActive() (string, error) {
	if m.getActiveFunc != nil {
		return m.getActiveFunc()
	}
	return "", nil
}

func (m *mockBinaryCache) CacheDir() string {
	if m.cacheDirFunc != nil {
		return m.cacheDirFunc()
	}
	return ""
}

func (m *mockBinaryCache) SymlinkPath() string {
	if m.symlinkPathFunc != nil {
		return m.symlinkPathFunc()
	}
	return ""
}

func (m *mockBinaryCache) SymlinkInfo() (*ports.SymlinkInfo, error) {
	if m.symlinkInfoFunc != nil {
		return m.symlinkInfoFunc()
	}
	return &ports.SymlinkInfo{}, nil
}

type mockValidatorKeyLoader struct {
	loadValidatorKeysFunc func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error)
}

func (m *mockValidatorKeyLoader) LoadValidatorKeys(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
	if m.loadValidatorKeysFunc != nil {
		return m.loadValidatorKeysFunc(ctx, opts)
	}
	return []ports.ValidatorKey{}, nil
}

type mockExportUseCase struct {
	executeFunc func(ctx context.Context, input interface{}) (interface{}, error)
}

func (m *mockExportUseCase) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return nil, nil
}

type mockStateManager struct {
	loadStateFunc     func(ctx context.Context) (*ports.UpgradeState, error)
	saveStateFunc     func(ctx context.Context, state *ports.UpgradeState) error
	deleteStateFunc   func(ctx context.Context) error
	stateExistsFunc   func(ctx context.Context) (bool, error)
	validateStateFunc func(state *ports.UpgradeState) error
	acquireLockFunc   func(ctx context.Context) error
	releaseLockFunc   func(ctx context.Context) error

	savedStates []*ports.UpgradeState
}

func (m *mockStateManager) LoadState(ctx context.Context) (*ports.UpgradeState, error) {
	if m.loadStateFunc != nil {
		return m.loadStateFunc(ctx)
	}
	return nil, nil
}

func (m *mockStateManager) SaveState(ctx context.Context, state *ports.UpgradeState) error {
	if m.saveStateFunc != nil {
		return m.saveStateFunc(ctx, state)
	}
	clone := *state
	m.savedStates = append(m.savedStates, &clone)
	return nil
}

func (m *mockStateManager) DeleteState(ctx context.Context) error {
	if m.deleteStateFunc != nil {
		return m.deleteStateFunc(ctx)
	}
	return nil
}

func (m *mockStateManager) StateExists(ctx context.Context) (bool, error) {
	if m.stateExistsFunc != nil {
		return m.stateExistsFunc(ctx)
	}
	return false, nil
}

func (m *mockStateManager) ValidateState(state *ports.UpgradeState) error {
	if m.validateStateFunc != nil {
		return m.validateStateFunc(state)
	}
	return nil
}

func (m *mockStateManager) AcquireLock(ctx context.Context) error {
	if m.acquireLockFunc != nil {
		return m.acquireLockFunc(ctx)
	}
	return nil
}

func (m *mockStateManager) ReleaseLock(ctx context.Context) error {
	if m.releaseLockFunc != nil {
		return m.releaseLockFunc(ctx)
	}
	return nil
}

type mockStateDetector struct {
	detectCurrentStageFunc   func(ctx context.Context, state *ports.UpgradeState) (ports.ResumableStage, error)
	detectProposalStatusFunc func(ctx context.Context, proposalID uint64) (string, error)
	detectChainStatusFunc    func(ctx context.Context) (string, error)
	detectValidatorVotesFunc func(ctx context.Context, proposalID uint64) ([]ports.ValidatorVoteState, error)
}

func (m *mockStateDetector) DetectCurrentStage(ctx context.Context, state *ports.UpgradeState) (ports.ResumableStage, error) {
	if m.detectCurrentStageFunc != nil {
		return m.detectCurrentStageFunc(ctx, state)
	}
	return state.Stage, nil
}

func (m *mockStateDetector) DetectProposalStatus(ctx context.Context, proposalID uint64) (string, error) {
	if m.detectProposalStatusFunc != nil {
		return m.detectProposalStatusFunc(ctx, proposalID)
	}
	return "unknown", nil
}

func (m *mockStateDetector) DetectChainStatus(ctx context.Context) (string, error) {
	if m.detectChainStatusFunc != nil {
		return m.detectChainStatusFunc(ctx)
	}
	return "running", nil
}

func (m *mockStateDetector) DetectValidatorVotes(ctx context.Context, proposalID uint64) ([]ports.ValidatorVoteState, error) {
	if m.detectValidatorVotesFunc != nil {
		return m.detectValidatorVotesFunc(ctx, proposalID)
	}
	return []ports.ValidatorVoteState{}, nil
}

type mockTransitioner struct {
	transitionToFunc        func(state *ports.UpgradeState, target ports.ResumableStage, reason string) error
	canTransitionFunc       func(from, to ports.ResumableStage) bool
	getValidTransitionsFunc func(from ports.ResumableStage) []ports.ResumableStage
}

func (m *mockTransitioner) TransitionTo(state *ports.UpgradeState, target ports.ResumableStage, reason string) error {
	if m.transitionToFunc != nil {
		return m.transitionToFunc(state, target, reason)
	}
	prev := state.Stage
	state.Stage = target
	state.StageHistory = append(state.StageHistory, ports.StageTransition{From: prev, To: target, Reason: reason, Timestamp: time.Now()})
	return nil
}

func (m *mockTransitioner) CanTransition(from, to ports.ResumableStage) bool {
	if m.canTransitionFunc != nil {
		return m.canTransitionFunc(from, to)
	}
	return true
}

func (m *mockTransitioner) GetValidTransitions(from ports.ResumableStage) []ports.ResumableStage {
	if m.getValidTransitionsFunc != nil {
		return m.getValidTransitionsFunc(from)
	}
	return []ports.ResumableStage{}
}
