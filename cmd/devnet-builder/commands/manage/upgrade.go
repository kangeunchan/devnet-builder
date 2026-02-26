package manage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application"
	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/internal/di"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/binary"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/cache"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/executor"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/filesystem"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/interactive"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/network"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/persistence"
	infrarpc "github.com/altuslabsxyz/devnet-builder/internal/infrastructure/rpc"
	"github.com/altuslabsxyz/devnet-builder/internal/output"
	"github.com/altuslabsxyz/devnet-builder/internal/paths"
	"github.com/altuslabsxyz/devnet-builder/types"
	"github.com/altuslabsxyz/devnet-builder/types/ctxconfig"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Default upgrade constants
const (
	DefaultHeightBuffer = 0 // 0 = auto-calculate based on block time
	DefaultVotingPeriod = 60 * time.Second
)

// UpgradeExecutionMode for upgrade command
type UpgradeExecutionMode string

const (
	UpgradeModeDocker UpgradeExecutionMode = "docker"
	UpgradeModeLocal  UpgradeExecutionMode = "local"
)

// Upgrade command flags
var (
	upgradeName          string
	upgradeImage         string
	upgradeBinary        string
	upgradeMode          string
	votingPeriod         string
	forceVotingPeriod    bool
	heightBuffer         int
	withExport           bool
	genesisDir           string
	upgradeNoInteractive bool
	upgradeVersion       string
	skipGovernance       bool

	// Resume-related flags
	upgradeResume       bool
	upgradeForceRestart bool
	upgradeResumeFrom   string
	upgradeClearState   bool
	upgradeShowStatus   bool
)

// NewUpgradeCmd creates the upgrade command.
func NewUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Perform a software upgrade on the running devnet",
		Long: `Perform a software upgrade on the running devnet using Cosmos SDK governance.

This command automates the complete upgrade process:
  1. Submit an expedited upgrade proposal
  2. Vote YES from all validators
  3. Wait for the upgrade height
  4. Switch to the new binary
  5. Verify chain resumes

With --skip-gov flag, skip governance and directly replace the binary:
  1. Stop all nodes
  2. Replace the binary
  3. Restart all nodes
  4. Verify chain resumes

Examples:
  # Upgrade to a new Docker image (with governance)
  devnet-builder upgrade --name v2.0.0-upgrade --image ghcr.io/stablelabs/stable:v2.0.0

  # Upgrade to a local binary (interactive selection)
  devnet-builder upgrade --name v2.0.0-upgrade --version v2.0.0
  → Then select "Use local binary" and browse to your binary

  # Skip governance and directly replace binary (quick testing)
  devnet-builder upgrade --skip-gov --version v2.0.0

  # Skip governance with interactive version selection
  devnet-builder upgrade --skip-gov

  # Upgrade with custom voting period (fallback if chain query fails)
  devnet-builder upgrade --name v2.0.0-upgrade --image ghcr.io/stablelabs/stable:v2.0.0 --voting-period 120s

  # Force custom voting period (override chain/plugin parameters)
  devnet-builder upgrade --name v2.0.0-upgrade --image ghcr.io/stablelabs/stable:v2.0.0 --voting-period 30s --force-voting-period

  # Upgrade and export state snapshots
  devnet-builder upgrade --name v2.0.0-upgrade --image ghcr.io/stablelabs/stable:v2.0.0 --with-export

  # Interactive mode (default) - select version interactively
  devnet-builder upgrade

  # Non-interactive mode with explicit version
  devnet-builder upgrade --no-interactive --name v2.0.0-upgrade --version v2.0.0

Resume options (for interrupted upgrades):
  # Check current upgrade state
  devnet-builder upgrade --show-status

  # Resume an interrupted upgrade
  devnet-builder upgrade --resume

  # Clear state and start fresh
  devnet-builder upgrade --clear-state

  # Force restart (ignore saved state)
  devnet-builder upgrade --force-restart --name v2.0.0-upgrade --version v2.0.0

  # Resume from a specific stage (advanced)
  devnet-builder upgrade --resume --resume-from SwitchingBinary`,
		RunE: runUpgrade,
	}

	// Version selection flags
	cmd.Flags().StringVarP(&upgradeName, "name", "n", "", "Upgrade handler name")
	cmd.Flags().StringVarP(&upgradeImage, "image", "i", "", "Target Docker image for upgrade")
	cmd.Flags().StringVar(&upgradeVersion, "version", "", "Target version (tag or branch/commit for building)")
	cmd.Flags().StringVarP(&upgradeMode, "mode", "m", "", "Execution mode: docker or local (default: from devnet metadata)")

	// Interactive mode flags
	cmd.Flags().BoolVar(&upgradeNoInteractive, "no-interactive", false, "Disable interactive mode")

	// Skip governance flag
	cmd.Flags().BoolVar(&skipGovernance, "skip-gov", false, "Skip governance proposal and directly replace binary (like old 'replace' command)")

	// Optional flags
	cmd.Flags().StringVar(&votingPeriod, "voting-period", "60s", "Expedited voting period duration")
	cmd.Flags().BoolVar(&forceVotingPeriod, "force-voting-period", false, "Force use of --voting-period value, ignoring on-chain parameters")
	cmd.Flags().IntVar(&heightBuffer, "height-buffer", DefaultHeightBuffer, "Blocks to add after voting period ends (0 = auto-calculate based on block time)")
	cmd.Flags().BoolVar(&withExport, "with-export", false, "Export state before and after upgrade")
	cmd.Flags().StringVar(&genesisDir, "genesis-dir", "", "Directory for genesis exports (default: <home>/devnet/genesis-snapshots)")

	// Resume flags (for interrupted upgrades)
	cmd.Flags().BoolVar(&upgradeResume, "resume", false, "Resume an interrupted upgrade from saved state")
	cmd.Flags().BoolVar(&upgradeForceRestart, "force-restart", false, "Ignore saved state and start a fresh upgrade")
	cmd.Flags().StringVar(&upgradeResumeFrom, "resume-from", "", "Resume from a specific stage (advanced: Initialized, ProposalSubmitted, Voting, WaitingForHeight, ChainHalted, SwitchingBinary, VerifyingResume)")
	cmd.Flags().BoolVar(&upgradeClearState, "clear-state", false, "Clear saved upgrade state and exit")
	cmd.Flags().BoolVar(&upgradeShowStatus, "show-status", false, "Show current upgrade state and exit")

	return cmd
}

// UpgradeResultJSON represents the JSON output for the upgrade command.
type UpgradeResultJSON struct {
	Status            string `json:"status"`
	UpgradeName       string `json:"upgrade_name"`
	ProposalID        uint64 `json:"proposal_id"`
	UpgradeHeight     int64  `json:"upgrade_height"`
	PostUpgradeHeight int64  `json:"post_upgrade_height"`
	NewBinary         string `json:"new_binary"`
	Duration          string `json:"duration"`
	PreGenesisPath    string `json:"pre_genesis_path,omitempty"`
	PostGenesisPath   string `json:"post_genesis_path,omitempty"`
}

type upgradeContext struct {
	cmd        *cobra.Command
	commandCtx context.Context
	ctx        context.Context
	homeDir    string
	jsonMode   bool
	logger     *output.Logger
}

type upgradeResolvedConfig struct {
	svc               *application.DevnetService
	metadata          *ports.DevnetMetadata
	networkModule     network.NetworkModule
	resolvedMode      UpgradeExecutionMode
	modeExplicitlySet bool
}

type upgradeBinaryResolution struct {
	selectedVersion         string
	selectedName            string
	customBinarySymlinkPath string
	cachedBuildResult       *dto.BuildOutput
	versionResolvedImage    string
	targetBinary            string
	targetImage             string
}

type upgradeGovernanceResolution struct {
	govParams    *ports.GovParams
	votingPeriod time.Duration
}

type upgradeExecutionResult struct {
	result *dto.ExecuteUpgradeOutput
}

var errUpgradeCancelled = errors.New("upgrade cancelled")

func runUpgrade(cmd *cobra.Command, args []string) error {
	upgradeCtx := newUpgradeContext(cmd)
	var cleanup func()
	upgradeCtx.ctx, cleanup = setupSignalHandling(upgradeCtx.commandCtx)
	defer cleanup()

	if upgradeClearState || upgradeShowStatus {
		return handleResumeOnlyOperations(upgradeCtx.ctx, upgradeCtx.homeDir, upgradeCtx.logger, upgradeCtx.jsonMode)
	}

	resolvedConfig, err := resolveUpgradeConfig(upgradeCtx)
	if err != nil {
		return err
	}

	binaryResolution, err := resolveBinarySource(upgradeCtx, resolvedConfig)
	if errors.Is(err, errUpgradeCancelled) {
		return nil
	}
	if err != nil {
		return err
	}

	governanceResolution, err := resolveGovernanceParams(upgradeCtx, resolvedConfig)
	if err != nil {
		return err
	}

	executionResult, err := executeUpgrade(upgradeCtx, resolvedConfig, binaryResolution, governanceResolution)
	if err != nil {
		return err
	}

	return reportResults(upgradeCtx, executionResult)
}

func newUpgradeContext(cmd *cobra.Command) *upgradeContext {
	commandCtx := cmd.Context()
	cfg := ctxconfig.FromContext(commandCtx)

	return &upgradeContext{
		cmd:        cmd,
		commandCtx: commandCtx,
		homeDir:    cfg.HomeDir(),
		jsonMode:   cfg.JSONMode(),
		logger:     output.DefaultLogger,
	}
}

func setupSignalHandling(commandCtx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(commandCtx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println()
		output.Warn("Upgrade interrupted. Current state:")
		lastUpgradeStageMu.RLock()
		stage := lastUpgradeStage
		lastUpgradeStageMu.RUnlock()
		if stage != "" {
			fmt.Printf("  Last stage: %s\n", stage)
		}
		output.Warn("The devnet may be in an intermediate state.")
		output.Info("Run 'devnet-builder status' to check chain health.")
		cancel()
	}()

	cleanup := func() {
		signal.Stop(sigChan)
		cancel()
	}

	return ctx, cleanup
}

func resolveUpgradeConfig(upgradeCtx *upgradeContext) (*upgradeResolvedConfig, error) {
	svc, err := application.GetService(upgradeCtx.homeDir)
	if err != nil {
		return nil, outputUpgradeError(fmt.Errorf("failed to initialize service: %w", err))
	}

	if !svc.DevnetExists() {
		err := fmt.Errorf("no devnet found at %s", upgradeCtx.homeDir)
		if upgradeCtx.jsonMode {
			return nil, outputUpgradeError(err)
		}
		return nil, err
	}

	metadata, err := svc.LoadMetadata(upgradeCtx.ctx)
	if err != nil {
		if upgradeCtx.jsonMode {
			return nil, outputUpgradeError(err)
		}
		return nil, err
	}

	if metadata.Status != ports.StateRunning {
		if upgradeCtx.jsonMode {
			return nil, outputUpgradeError(fmt.Errorf("devnet is not running"))
		}
		return nil, fmt.Errorf("devnet is not running\nStart it with 'devnet-builder start'")
	}

	networkModule, err := network.Get(metadata.BlockchainNetwork)
	if err != nil {
		return nil, outputUpgradeError(fmt.Errorf("failed to get network module: %w", err))
	}

	resolvedMode, modeExplicitlySet, err := resolveUpgradeExecutionMode(metadata.ExecutionMode)
	if err != nil {
		return nil, err
	}

	if !upgradeCtx.jsonMode {
		warnUpgradeModeMismatch(resolvedMode, modeExplicitlySet, metadata.ExecutionMode)
	}

	return &upgradeResolvedConfig{
		svc:               svc,
		metadata:          metadata,
		networkModule:     networkModule,
		resolvedMode:      resolvedMode,
		modeExplicitlySet: modeExplicitlySet,
	}, nil
}

func resolveUpgradeExecutionMode(metadataMode types.ExecutionMode) (UpgradeExecutionMode, bool, error) {
	resolvedMode := UpgradeExecutionMode(metadataMode)
	modeExplicitlySet := false

	if upgradeMode != "" {
		switch UpgradeExecutionMode(upgradeMode) {
		case UpgradeModeDocker, UpgradeModeLocal:
			resolvedMode = UpgradeExecutionMode(upgradeMode)
			modeExplicitlySet = true
		default:
			return "", false, fmt.Errorf("invalid mode %q: must be 'docker' or 'local'", upgradeMode)
		}
	}

	return resolvedMode, modeExplicitlySet, nil
}

func warnUpgradeModeMismatch(resolvedMode UpgradeExecutionMode, modeExplicitlySet bool, metadataMode types.ExecutionMode) {
	if resolvedMode == UpgradeModeDocker && upgradeBinary != "" && !modeExplicitlySet {
		output.Warn("Devnet was started in docker mode but --binary was provided.")
		output.Warn("Use --image for docker mode, or --mode local to switch modes.")
	}
	if resolvedMode == UpgradeModeLocal && upgradeImage != "" && !modeExplicitlySet {
		output.Warn("Devnet was started in local mode but --image was provided.")
		output.Warn("Use --binary for local mode, or --mode docker to switch modes.")
	}
	if modeExplicitlySet && resolvedMode != UpgradeExecutionMode(metadataMode) {
		output.Warn("Switching execution mode from %s to %s.", metadataMode, resolvedMode)
		output.Warn("The devnet will continue in %s mode after this upgrade.", resolvedMode)
	}
}

func resolveBinarySource(upgradeCtx *upgradeContext, resolvedConfig *upgradeResolvedConfig) (*upgradeBinaryResolution, error) {
	selectedName := upgradeName
	selectedVersion := upgradeVersion
	customBinarySymlinkPath := ""

	if shouldRunUpgradeInteractiveSelection(upgradeCtx.jsonMode) {
		selection, err := RunInteractiveVersionSelectionWithMode(
			upgradeCtx.ctx,
			upgradeCtx.cmd,
			false,
			true,
			"",
			skipGovernance,
			resolvedConfig.metadata.BlockchainNetwork,
		)
		if err != nil {
			if interactive.IsCancellation(err) {
				fmt.Println("Operation cancelled.")
				return nil, errUpgradeCancelled
			}
			return nil, err
		}

		selectedVersion = selection.StartVersion
		selectedName = resolveUpgradeName(selection)
		if selection.BinarySource.IsLocal() && selection.BinarySource.SelectedPath != "" {
			customBinarySymlinkPath = selection.BinarySource.SelectedPath
		}
	}

	if err := validateUpgradeSourceInputs(selectedVersion, selectedName); err != nil {
		return nil, err
	}

	cachedBuildResult, versionResolvedImage, err := resolveUpgradeBuildTarget(upgradeCtx, resolvedConfig, selectedVersion)
	if err != nil {
		return nil, err
	}

	customBinarySymlinkPath, err = resolveUpgradeLocalBinaryPath(
		upgradeCtx,
		resolvedConfig,
		cachedBuildResult,
		customBinarySymlinkPath,
	)
	if err != nil {
		return nil, err
	}

	targetBinary, targetImage := buildUpgradeTargets(customBinarySymlinkPath, versionResolvedImage)

	return &upgradeBinaryResolution{
		selectedVersion:         selectedVersion,
		selectedName:            selectedName,
		customBinarySymlinkPath: customBinarySymlinkPath,
		cachedBuildResult:       cachedBuildResult,
		versionResolvedImage:    versionResolvedImage,
		targetBinary:            targetBinary,
		targetImage:             targetImage,
	}, nil
}

func shouldRunUpgradeInteractiveSelection(jsonMode bool) bool {
	return !upgradeNoInteractive && !jsonMode && upgradeImage == "" && upgradeBinary == ""
}

func resolveUpgradeName(selection *interactive.SelectionConfig) string {
	if upgradeName != "" {
		return upgradeName
	}
	if selection.UpgradeName != "" {
		return selection.UpgradeName
	}

	versionForName := selection.StartVersion
	if selection.StartIsCustomRef && strings.Contains(versionForName, "/") {
		parts := strings.Split(versionForName, "/")
		versionForName = parts[len(parts)-1]
	}
	return versionForName + "-upgrade"
}

func validateUpgradeSourceInputs(selectedVersion, selectedName string) error {
	if upgradeBinary != "" {
		return fmt.Errorf(`the --binary flag has been removed in favor of interactive binary selection

When you run 'devnet-builder upgrade' in interactive mode, you will be prompted to:
1. Choose between using a local binary or downloading from GitHub releases
2. If you select "local binary", you can browse your filesystem with Tab autocomplete

Migration guide:
  • Interactive mode (recommended):
      devnet-builder upgrade
      → Select "Use local binary (browse filesystem)"
      → Navigate to your binary using Tab autocomplete

  • Non-interactive mode with environment variable:
      export DEVNET_BINARY_PATH=/path/to/your/binary
      devnet-builder upgrade --no-interactive --name upgrade-name --version v1.2.3

  • Docker mode (unchanged):
      devnet-builder upgrade --mode docker --image your-image:tag --name upgrade-name

For more information, see: https://github.com/altuslabsxyz/devnet-builder/blob/main/docs/MIGRATION.md`)
	}

	if upgradeImage == "" && selectedVersion == "" {
		return fmt.Errorf("either --image or --version must be provided (or use interactive mode)")
	}

	if selectedName == "" && !skipGovernance {
		return fmt.Errorf("upgrade name is required (--name or interactive mode)")
	}

	return nil
}

func resolveUpgradeBuildTarget(
	upgradeCtx *upgradeContext,
	resolvedConfig *upgradeResolvedConfig,
	selectedVersion string,
) (*dto.BuildOutput, string, error) {
	if selectedVersion == "" || upgradeImage != "" || upgradeBinary != "" {
		return nil, "", nil
	}

	if resolvedConfig.resolvedMode == UpgradeModeDocker && isStandardVersionTag(selectedVersion) {
		dockerImage := resolvedConfig.networkModule.DockerImage()
		versionResolvedImage := fmt.Sprintf("%s:%s", dockerImage, selectedVersion)
		upgradeCtx.logger.Info("Using docker image for version %s: %s", selectedVersion, versionResolvedImage)
		return nil, versionResolvedImage, nil
	}

	buildResult, err := buildBinaryForUpgrade(
		upgradeCtx.ctx,
		resolvedConfig.metadata.BlockchainNetwork,
		selectedVersion,
		resolvedConfig.metadata.NetworkName,
		upgradeCtx.homeDir,
		upgradeCtx.logger,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to pre-build binary: %w", err)
	}

	commitShort := buildResult.CommitHash
	if len(commitShort) > 12 {
		commitShort = commitShort[:12]
	}
	upgradeCtx.logger.Success("Binary pre-built and cached (commit: %s)", commitShort)

	return buildResult, "", nil
}

func resolveUpgradeLocalBinaryPath(
	upgradeCtx *upgradeContext,
	resolvedConfig *upgradeResolvedConfig,
	cachedBuildResult *dto.BuildOutput,
	customBinarySymlinkPath string,
) (string, error) {
	if resolvedConfig.resolvedMode != UpgradeModeLocal {
		return customBinarySymlinkPath, nil
	}

	if cachedBuildResult != nil {
		upgradeCtx.logger.Debug("Using pre-built binary from custom ref: %s", cachedBuildResult.BinaryPath)
		return customBinarySymlinkPath, nil
	}

	if customBinarySymlinkPath == "" {
		selectedPath, err := selectBinaryForUpgrade(
			upgradeCtx.ctx,
			resolvedConfig.metadata.NetworkName,
			resolvedConfig.metadata.BlockchainNetwork,
			upgradeCtx.homeDir,
			upgradeCtx.logger,
		)
		if err != nil {
			return "", err
		}

		if selectedPath == "" {
			cacheDir := paths.BinaryCachePath(upgradeCtx.homeDir)
			return "", fmt.Errorf("no cached binaries found for upgrade\nCache directory: %s\nUse --binary flag to specify a binary, or deploy/build a binary first", cacheDir)
		}

		return selectedPath, nil
	}

	upgradeCtx.logger.Success("Using selected binary: %s", customBinarySymlinkPath)
	return customBinarySymlinkPath, nil
}

func buildUpgradeTargets(customBinarySymlinkPath, versionResolvedImage string) (string, string) {
	targetBinary := customBinarySymlinkPath
	if targetBinary == "" && upgradeBinary != "" {
		targetBinary = upgradeBinary
	}
	targetImage := upgradeImage
	if versionResolvedImage != "" {
		targetImage = versionResolvedImage
	}
	return targetBinary, targetImage
}

func resolveGovernanceParams(
	upgradeCtx *upgradeContext,
	resolvedConfig *upgradeResolvedConfig,
) (*upgradeGovernanceResolution, error) {
	var govParams *ports.GovParams
	var votingPeriodDuration time.Duration

	if skipGovernance {
		if !upgradeCtx.jsonMode {
			upgradeCtx.logger.Warn("Skipping governance proposal (--skip-gov mode)")
			upgradeCtx.logger.Warn("This will directly replace the binary WITHOUT governance upgrade.")
			upgradeCtx.logger.Warn("Chain state must be compatible with the new version.")
			fmt.Println()
		}
		return &upgradeGovernanceResolution{
			govParams:    nil,
			votingPeriod: 0,
		}, nil
	}

	if forceVotingPeriod {
		upgradeCtx.logger.Info("Using forced voting period from --voting-period flag...")
		parsedVotingPeriod, err := time.ParseDuration(votingPeriod)
		if err != nil {
			return nil, fmt.Errorf("invalid voting period: %w", err)
		}
		upgradeCtx.logger.Info("Forced expedited voting period: %s", parsedVotingPeriod)
		return &upgradeGovernanceResolution{
			govParams:    nil,
			votingPeriod: parsedVotingPeriod,
		}, nil
	}

	upgradeCtx.logger.Info("Fetching governance parameters from chain...")
	rpcHost := "localhost"
	rpcPort := 26657

	tempFactory := di.NewInfrastructureFactory(upgradeCtx.homeDir, upgradeCtx.logger).
		WithNetworkModule(resolvedConfig.networkModule)
	rpcClient := tempFactory.CreateRPCClient(rpcHost, rpcPort)

	if cosmosClient, ok := rpcClient.(*infrarpc.CosmosRPCClient); ok {
		if pluginModule, ok := resolvedConfig.networkModule.(infrarpc.NetworkPluginModule); ok {
			rpcClient = cosmosClient.WithPlugin(pluginModule, resolvedConfig.metadata.NetworkName)
		}
	}

	var err error
	govParams, err = rpcClient.GetGovParams(upgradeCtx.ctx)
	if err != nil {
		upgradeCtx.logger.Debug("Failed to fetch gov params, using CLI flag value: %v", err)
		parsedVotingPeriod, parseErr := time.ParseDuration(votingPeriod)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid voting period: %w", parseErr)
		}
		govParams = &ports.GovParams{
			ExpeditedVotingPeriod: parsedVotingPeriod,
		}
	}

	votingPeriodDuration = govParams.ExpeditedVotingPeriod
	upgradeCtx.logger.Info("Using expedited voting period: %s", votingPeriodDuration)

	return &upgradeGovernanceResolution{
		govParams:    govParams,
		votingPeriod: votingPeriodDuration,
	}, nil
}

func executeUpgrade(
	upgradeCtx *upgradeContext,
	resolvedConfig *upgradeResolvedConfig,
	binaryResolution *upgradeBinaryResolution,
	governanceResolution *upgradeGovernanceResolution,
) (*upgradeExecutionResult, error) {
	if !upgradeCtx.jsonMode {
		if skipGovernance {
			printSkipGovPlan(
				string(resolvedConfig.resolvedMode),
				binaryResolution.targetImage,
				binaryResolution.targetBinary,
				binaryResolution.cachedBuildResult,
				resolvedConfig.metadata,
			)
		} else {
			printUpgradePlan(
				binaryResolution.selectedName,
				string(resolvedConfig.resolvedMode),
				binaryResolution.targetImage,
				binaryResolution.targetBinary,
				binaryResolution.cachedBuildResult,
				governanceResolution.votingPeriod,
				resolvedConfig.metadata,
			)
		}
	}

	factory := di.NewInfrastructureFactory(upgradeCtx.homeDir, upgradeCtx.logger).
		WithNetworkModule(resolvedConfig.networkModule).
		WithDockerMode(resolvedConfig.resolvedMode == UpgradeModeDocker)

	container, err := factory.WireContainer()
	if err != nil {
		return nil, outputUpgradeError(fmt.Errorf("failed to initialize: %w", err))
	}

	resumeState, err := checkForExistingUpgradeState(upgradeCtx.ctx, upgradeCtx.homeDir, upgradeCtx.logger, upgradeCtx.jsonMode)
	if err != nil {
		if upgradeCtx.jsonMode {
			return nil, outputUpgradeError(err)
		}
		return nil, err
	}

	input := dto.ExecuteUpgradeInput{
		HomeDir:        upgradeCtx.homeDir,
		UpgradeName:    binaryResolution.selectedName,
		TargetBinary:   binaryResolution.targetBinary,
		TargetImage:    binaryResolution.targetImage,
		TargetVersion:  binaryResolution.selectedVersion,
		VotingPeriod:   governanceResolution.votingPeriod,
		HeightBuffer:   heightBuffer,
		UpgradeHeight:  0,
		WithExport:     withExport,
		GenesisDir:     genesisDir,
		Mode:           types.ExecutionMode(resolvedConfig.resolvedMode),
		SkipGovernance: skipGovernance,
	}

	if binaryResolution.cachedBuildResult != nil {
		input.CachePath = binaryResolution.cachedBuildResult.BinaryPath
		input.CommitHash = binaryResolution.cachedBuildResult.CommitHash
		input.CacheRef = binaryResolution.cachedBuildResult.CacheRef
		input.TargetBinary = ""
	}

	if !upgradeCtx.jsonMode {
		if resumeState != nil {
			fmt.Printf("[1/6] %s (resuming from %s)\n", color.CyanString("Verifying devnet status..."), resumeState.Stage)
		} else {
			fmt.Printf("[1/6] %s\n", color.CyanString("Verifying devnet status..."))
		}
	}

	result, err := container.ResumableExecuteUpgradeUseCase().Execute(upgradeCtx.ctx, input, resumeState)
	if err != nil {
		if upgradeCtx.jsonMode {
			return nil, outputUpgradeError(err)
		}
		return nil, err
	}

	if result.Success {
		resolvedConfig.metadata.CurrentVersion = binaryResolution.selectedVersion
		resolvedConfig.metadata.ExecutionMode = types.ExecutionMode(resolvedConfig.resolvedMode)
		if err := resolvedConfig.svc.SaveMetadata(upgradeCtx.ctx, resolvedConfig.metadata); err != nil {
			upgradeCtx.logger.Warn("Failed to update metadata: %v", err)
		}
	}

	return &upgradeExecutionResult{
		result: result,
	}, nil
}

func reportResults(upgradeCtx *upgradeContext, executionResult *upgradeExecutionResult) error {
	if upgradeCtx.jsonMode {
		return outputUpgradeJSON(executionResult.result)
	}
	return outputUpgradeText(executionResult.result)
}

// selectBinaryForUpgrade orchestrates binary selection from cache for upgrade command.
// This is simpler than selectBinaryForDeployment because upgrade doesn't build from source.
func selectBinaryForUpgrade(
	ctx context.Context,
	networkName string,
	blockchain string,
	homeDir string,
	logger *output.Logger,
) (string, error) {
	// Setup components (same as deploy)
	fs := filesystem.NewOSFileSystem()
	scanner := cache.NewBinaryScanner(fs)

	exec := executor.NewOSCommandExecutor()
	detector := binary.NewVersionDetectorAdapter(exec, 5*time.Second)
	validator := binary.NewBinaryValidator(detector)

	prompter := interactive.NewPrompterAdapter()
	selector := interactive.NewBinarySelector(prompter)

	// Scan cache for binaries
	binaryName := blockchain + "d" // e.g., "stable" → "stabled"
	cacheDir := paths.BinaryCachePath(homeDir)

	// Debug: log what we're searching for
	logger.Debug("Scanning cache for binaries: network=%q, blockchain=%q, binaryName=%q, cacheDir=%q",
		networkName, blockchain, binaryName, cacheDir)

	var scannedBinaries []cache.CachedBinaryMetadata
	var err error

	if networkName != "" {
		// Scan specific network directory
		scannedBinaries, err = scanner.ScanCachedBinaries(ctx, cacheDir, networkName, binaryName)
		if err != nil {
			return "", fmt.Errorf("failed to scan cache: %w", err)
		}
		logger.Debug("Found %d binaries in network %q", len(scannedBinaries), networkName)
	}

	// Fallback: if no binaries found with specific network, scan ALL networks
	// This handles edge cases where NetworkName in metadata is empty or mismatched
	if len(scannedBinaries) == 0 {
		if networkName != "" {
			logger.Debug("No binaries found in network %q, scanning all networks...", networkName)
		} else {
			logger.Debug("Network not specified, scanning all networks...")
		}

		scannedBinaries, err = scanner.ScanAllNetworks(ctx, cacheDir, binaryName)
		if err != nil {
			return "", fmt.Errorf("failed to scan all networks: %w", err)
		}
		logger.Debug("Found %d binaries across all networks", len(scannedBinaries))
	}

	// Validate binaries concurrently
	validBinaries, invalidBinaries := filterValidBinariesForUpgradeWithDiagnostics(ctx, scannedBinaries, validator, logger)

	// If no valid binaries found, provide detailed diagnostics
	if len(validBinaries) == 0 {
		if len(scannedBinaries) > 0 {
			// Binaries were found but all failed validation - show WHY
			logger.Warn("Found %d cached binaries but all failed validation:", len(scannedBinaries))
			for i := range invalidBinaries {
				logger.Warn("  - %s: %s", invalidBinaries[i].binary.Path, invalidBinaries[i].reason)
			}
			// Offer to clean up invalid cache entries
			logger.Warn("Run 'devnet-builder cache clean' to remove invalid entries")
			return "", fmt.Errorf("all %d cached binaries failed validation (see warnings above)", len(scannedBinaries))
		}
		logger.Debug("No cached binaries found for %q in cache directory: %s", binaryName, cacheDir)
		return "", nil // Empty result indicates no cache
	}

	// Run selection with appropriate options
	// Note: Upgrade doesn't have "Build from source" option
	opts := interactive.BinarySelectionOptions{
		AllowBuildFromSource: false,                               // Upgrade must use existing binary
		AutoSelectSingle:     true,                                // CLARIFICATION 1: Auto-select single binary
		IsInteractive:        interactive.IsTerminalInteractive(), // EC-004: TTY detection
	}

	result, err := selector.RunBinarySelectionFlow(ctx, validBinaries, opts)
	if err != nil {
		return "", fmt.Errorf("binary selection failed: %w", err)
	}

	// User cancelled
	if result.WasCancelled {
		return "", fmt.Errorf("selection cancelled by user")
	}

	// No binary selected (shouldn't happen since AllowBuildFromSource=false)
	if result.SelectedBinary == nil {
		return "", fmt.Errorf("no binary selected")
	}

	// Binary selected from cache
	// EC-002: Single binary was auto-selected (log for transparency)
	if len(validBinaries) == 1 {
		logger.Info("Using cached binary: %s %s (%s)",
			result.SelectedBinary.Name,
			result.SelectedBinary.Version,
			result.SelectedBinary.CommitHashShort)
	} else {
		logger.Success("Selected binary: %s %s (%s)",
			result.SelectedBinary.Name,
			result.SelectedBinary.Version,
			result.SelectedBinary.CommitHashShort)
	}

	return result.BinaryPath, nil
}

// invalidBinaryInfo holds information about a binary that failed validation.
type invalidBinaryInfo struct {
	binary cache.CachedBinaryMetadata
	reason string
}

// filterValidBinariesForUpgradeWithDiagnostics validates binaries and returns both valid and invalid lists.
// This provides better diagnostics for debugging cache issues.
func filterValidBinariesForUpgradeWithDiagnostics(
	ctx context.Context,
	binaries []cache.CachedBinaryMetadata,
	validator *binary.BinaryValidator,
	logger *output.Logger,
) ([]cache.CachedBinaryMetadata, []invalidBinaryInfo) {
	if len(binaries) == 0 {
		return []cache.CachedBinaryMetadata{}, []invalidBinaryInfo{}
	}

	// Validate concurrently for performance
	type validationResult struct {
		binary cache.CachedBinaryMetadata
		err    error
	}

	results := make(chan validationResult, len(binaries))
	var wg sync.WaitGroup

	for i := range binaries {
		wg.Add(1)
		go func(b cache.CachedBinaryMetadata) {
			defer wg.Done()

			// Validate and enrich with version info
			enriched, err := validator.ValidateAndEnrichMetadata(ctx, &b)
			results <- validationResult{
				binary: *enriched,
				err:    err,
			}
		}(binaries[i])
	}

	// Wait for all validations to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect valid and invalid binaries
	var validBinaries []cache.CachedBinaryMetadata
	var invalidBinaries []invalidBinaryInfo
	for result := range results {
		if result.err != nil {
			// EC-007: Corrupted/invalid binary - collect for diagnostics
			invalidBinaries = append(invalidBinaries, invalidBinaryInfo{
				binary: result.binary,
				reason: result.err.Error(),
			})
			continue
		}
		if result.binary.IsValid {
			validBinaries = append(validBinaries, result.binary)
		} else {
			invalidBinaries = append(invalidBinaries, invalidBinaryInfo{
				binary: result.binary,
				reason: result.binary.ValidationError,
			})
		}
	}

	// Sort by modification time descending (most recent first)
	sort.Slice(validBinaries, func(i, j int) bool {
		return validBinaries[i].ModTime.After(validBinaries[j].ModTime)
	})

	return validBinaries, invalidBinaries
}

// isStandardVersionTag checks if a string looks like a standard version tag (e.g., v1.2.3).
func isStandardVersionTag(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Standard version tags start with 'v' followed by a digit
	if s[0] == 'v' && len(s) > 1 && s[1] >= '0' && s[1] <= '9' {
		return true
	}
	return false
}

func printUpgradePlan(name, mode, targetImage, targetBinary string, cached *dto.BuildOutput, votingPeriod time.Duration, metadata *ports.DevnetMetadata) {
	output.Bold("Upgrade Plan")
	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Printf("Upgrade Name:     %s\n", name)
	fmt.Printf("ExecutionMode:             %s\n", mode)
	if targetImage != "" {
		fmt.Printf("Target Image:     %s\n", targetImage)
	} else if targetBinary != "" {
		fmt.Printf("Target Binary:    %s\n", targetBinary)
	} else if cached != nil {
		fmt.Printf("Target Binary:    %s (cached)\n", cached.BinaryPath)
	}
	fmt.Printf("Voting Period:    %s\n", votingPeriod)
	if heightBuffer == 0 {
		fmt.Printf("Height Buffer:    auto-calculate (based on block time)\n")
	} else {
		fmt.Printf("Height Buffer:    %d blocks (manual)\n", heightBuffer)
	}
	fmt.Printf("Upgrade Height:   auto-calculate\n")
	fmt.Printf("Validators:       %d\n", metadata.NumValidators)
	fmt.Println()
}

func printSkipGovPlan(mode, targetImage, targetBinary string, cached *dto.BuildOutput, metadata *ports.DevnetMetadata) {
	output.Bold("Binary Replacement Plan (--skip-gov)")
	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Printf("ExecutionMode:    %s\n", mode)
	if targetImage != "" {
		fmt.Printf("Target Image:     %s\n", targetImage)
	} else if targetBinary != "" {
		fmt.Printf("Target Binary:    %s\n", targetBinary)
	} else if cached != nil {
		fmt.Printf("Target Binary:    %s (cached)\n", cached.BinaryPath)
	}
	fmt.Printf("Current Version:  %s\n", metadata.CurrentVersion)
	fmt.Printf("Validators:       %d\n", metadata.NumValidators)
	fmt.Println()
}

var (
	lastUpgradeStage   string
	lastUpgradeStageMu sync.RWMutex
)

// handleResumeOnlyOperations handles --clear-state and --show-status flags
// that don't require the full upgrade flow.
func handleResumeOnlyOperations(ctx context.Context, homeDir string, logger *output.Logger, jsonMode bool) error {
	stateManager := persistence.NewFileUpgradeStateManager(homeDir)

	if upgradeClearState {
		// Clear state and exit
		exists, err := stateManager.StateExists(ctx)
		if err != nil {
			return fmt.Errorf("failed to check state: %w", err)
		}
		if !exists {
			if jsonMode {
				return outputUpgradeStateJSON(nil, "no_state", "No upgrade state to clear")
			}
			logger.Info("No upgrade state to clear")
			return nil
		}

		if err := stateManager.DeleteState(ctx); err != nil {
			return fmt.Errorf("failed to clear state: %w", err)
		}

		if jsonMode {
			return outputUpgradeStateJSON(nil, "cleared", "Upgrade state cleared")
		}
		logger.Success("Upgrade state cleared successfully")
		return nil
	}

	if upgradeShowStatus {
		// Show state and exit
		state, err := stateManager.LoadState(ctx)
		if err != nil {
			if _, ok := err.(*ports.StateCorruptionError); ok {
				if jsonMode {
					return outputUpgradeStateJSON(nil, "corrupted", err.Error())
				}
				logger.Error("Upgrade state is corrupted: %v", err)
				logger.Info("Use --clear-state to remove the corrupted state file")
				return nil
			}
			return fmt.Errorf("failed to load state: %w", err)
		}

		if state == nil {
			if jsonMode {
				return outputUpgradeStateJSON(nil, "no_state", "No upgrade in progress")
			}
			logger.Info("No upgrade in progress")
			return nil
		}

		if jsonMode {
			return outputUpgradeStateJSON(state, "in_progress", "")
		}

		printUpgradeState(state, logger)
		return nil
	}

	return nil
}

// printUpgradeState displays the current upgrade state in a human-readable format.
func printUpgradeState(state *ports.UpgradeState, logger *output.Logger) {
	output.Bold("Upgrade State")
	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Printf("  Upgrade Name:     %s\n", state.UpgradeName)
	fmt.Printf("  Current Stage:    %s\n", state.Stage)
	fmt.Printf("  Mode:             %s\n", state.Mode)
	fmt.Printf("  Skip Governance:  %t\n", state.SkipGovernance)

	if state.ProposalID > 0 {
		fmt.Printf("  Proposal ID:      %d\n", state.ProposalID)
	}
	if state.UpgradeHeight > 0 {
		fmt.Printf("  Upgrade Height:   %d\n", state.UpgradeHeight)
	}
	if state.TargetVersion != "" {
		fmt.Printf("  Target Version:   %s\n", state.TargetVersion)
	}
	if state.TargetBinary != "" {
		fmt.Printf("  Target Binary:    %s\n", state.TargetBinary)
	}
	if state.TargetImage != "" {
		fmt.Printf("  Target Image:     %s\n", state.TargetImage)
	}
	if state.Error != "" {
		fmt.Printf("  Error:            %s\n", color.RedString(state.Error))
	}

	fmt.Printf("  Created At:       %s\n", state.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Updated At:       %s\n", state.UpdatedAt.Format(time.RFC3339))

	// Show stage history summary
	if len(state.StageHistory) > 0 {
		fmt.Println()
		fmt.Println("  Stage History:")
		for i, h := range state.StageHistory {
			if i == 0 {
				fmt.Printf("    %d. %s (initial)\n", i+1, h.To)
			} else {
				fmt.Printf("    %d. %s → %s\n", i+1, h.From, h.To)
			}
		}
	}

	fmt.Println("─────────────────────────────────────────────────────────")

	// Provide guidance based on state
	if state.Stage.IsTerminal() {
		if state.Stage == ports.ResumableStageCompleted {
			logger.Success("Upgrade completed successfully!")
			logger.Info("Use --clear-state to remove the state file")
		} else if state.Stage == ports.ResumableStageFailed {
			logger.Warn("Upgrade failed: %s", state.Error)
			logger.Info("Use --force-restart to start a fresh upgrade, or --clear-state to clear state")
		} else if state.Stage == ports.ResumableStageProposalRejected {
			logger.Warn("Proposal was rejected")
			logger.Info("Use --force-restart to start a fresh upgrade, or --clear-state to clear state")
		}
	} else {
		logger.Info("Upgrade is in progress at stage: %s", state.Stage)
		logger.Info("Use --resume to continue the upgrade, or --force-restart to start fresh")
	}
}

// UpgradeStateJSON represents the JSON output for upgrade state operations.
type UpgradeStateJSON struct {
	Status  string              `json:"status"`
	Message string              `json:"message,omitempty"`
	State   *ports.UpgradeState `json:"state,omitempty"`
}

// outputUpgradeStateJSON outputs the upgrade state as JSON.
func outputUpgradeStateJSON(state *ports.UpgradeState, status, message string) error {
	result := UpgradeStateJSON{
		Status:  status,
		Message: message,
		State:   state,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

// checkForExistingUpgradeState checks if there's an existing upgrade state and handles it.
// Returns the state to resume from, or nil to start fresh.
func checkForExistingUpgradeState(ctx context.Context, homeDir string, logger *output.Logger, jsonMode bool) (*ports.UpgradeState, error) {
	stateManager := persistence.NewFileUpgradeStateManager(homeDir)

	// Check for existing state
	state, err := stateManager.LoadState(ctx)
	if err != nil {
		if _, ok := err.(*ports.StateCorruptionError); ok {
			if jsonMode {
				return nil, fmt.Errorf("upgrade state is corrupted: %w (use --clear-state to remove)", err)
			}
			logger.Warn("Upgrade state file is corrupted: %v", err)
			logger.Info("Use --clear-state to remove the corrupted state, or --force-restart to start fresh")
			return nil, fmt.Errorf("upgrade state is corrupted")
		}
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	// No existing state
	if state == nil {
		if upgradeResume {
			return nil, fmt.Errorf("no upgrade in progress to resume")
		}
		return nil, nil // Start fresh
	}

	// Handle --force-restart: clear state and start fresh
	if upgradeForceRestart {
		if err := stateManager.DeleteState(ctx); err != nil {
			logger.Warn("Failed to clear existing state: %v", err)
		} else {
			logger.Info("Cleared existing upgrade state (--force-restart)")
		}
		return nil, nil // Start fresh
	}

	// Existing state found - prompt user or require --resume flag
	if state.Stage.IsTerminal() {
		// Terminal state - need explicit action
		if state.Stage == ports.ResumableStageCompleted {
			if !jsonMode {
				logger.Success("Previous upgrade completed successfully!")
				logger.Info("Use --clear-state to remove the state file, or --force-restart to start a new upgrade")
			}
			return nil, fmt.Errorf("previous upgrade completed (use --force-restart to start a new upgrade)")
		}

		if !jsonMode {
			logger.Warn("Previous upgrade %s at stage: %s", state.Stage, state.Error)
			logger.Info("Use --force-restart to start a fresh upgrade, or --clear-state to clear state")
		}
		return nil, fmt.Errorf("previous upgrade failed (use --force-restart to start a new upgrade)")
	}

	// In-progress state - prompt for resume
	if !upgradeResume {
		if !jsonMode {
			logger.Warn("Found existing upgrade in progress:")
			logger.Warn("  Upgrade: %s, Stage: %s", state.UpgradeName, state.Stage)
			logger.Info("Use --resume to continue, or --force-restart to start fresh")
		}
		return nil, fmt.Errorf("upgrade already in progress (use --resume to continue)")
	}

	// --resume flag set - validate resume-from stage if specified
	if upgradeResumeFrom != "" {
		targetStage := ports.ResumableStage(upgradeResumeFrom)
		if !isValidStage(targetStage) {
			return nil, fmt.Errorf("invalid stage: %s", upgradeResumeFrom)
		}
		// Override the current stage (advanced feature)
		state.Stage = targetStage
		logger.Info("Resuming from override stage: %s", targetStage)
	}

	logger.Info("Resuming upgrade: %s from stage: %s", state.UpgradeName, state.Stage)
	return state, nil
}

// isValidStage checks if a stage string is a valid ResumableStage.
func isValidStage(stage ports.ResumableStage) bool {
	validStages := []ports.ResumableStage{
		ports.ResumableStageInitialized,
		ports.ResumableStageProposalSubmitted,
		ports.ResumableStageVoting,
		ports.ResumableStageWaitingForHeight,
		ports.ResumableStageChainHalted,
		ports.ResumableStageSwitchingBinary,
		ports.ResumableStageVerifyingResume,
		ports.ResumableStageCompleted,
		ports.ResumableStageFailed,
		ports.ResumableStageProposalRejected,
	}
	for _, v := range validStages {
		if stage == v {
			return true
		}
	}
	return false
}

func outputUpgradeText(result *dto.ExecuteUpgradeOutput) error {
	if result.Error != nil {
		output.Error("Upgrade failed: %v", result.Error)
		return result.Error
	}

	fmt.Println()
	output.Success("Upgrade completed successfully!")
	fmt.Println()
	output.Bold("Upgrade Summary")
	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Printf("  Status:           %s\n", color.GreenString("SUCCESS"))
	fmt.Printf("  Proposal ID:      %d\n", result.ProposalID)
	fmt.Printf("  Upgrade Height:   %d\n", result.UpgradeHeight)
	fmt.Printf("  Post-Upgrade:     %d (chain resumed)\n", result.PostUpgradeHeight)
	fmt.Printf("  New Binary:       %s\n", result.NewBinary)
	fmt.Printf("  Total Duration:   %s\n", result.Duration.Round(time.Second))

	// Show genesis export paths if available
	if result.PreGenesisPath != "" || result.PostGenesisPath != "" {
		fmt.Println()
		output.Bold("Genesis Snapshots:")
		if result.PreGenesisPath != "" {
			fmt.Printf("  Pre-Upgrade:      %s\n", result.PreGenesisPath)
		}
		if result.PostGenesisPath != "" {
			fmt.Printf("  Post-Upgrade:     %s\n", result.PostGenesisPath)
		}
	}

	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Println()
	output.Info("Use 'devnet-builder status' to verify chain health")
	fmt.Println()
	return nil
}

func outputUpgradeJSON(result *dto.ExecuteUpgradeOutput) error {
	if result.Error != nil {
		return outputUpgradeError(result.Error)
	}

	jsonResult := UpgradeResultJSON{
		Status:            "success",
		UpgradeName:       result.NewBinary,
		ProposalID:        result.ProposalID,
		UpgradeHeight:     result.UpgradeHeight,
		PostUpgradeHeight: result.PostUpgradeHeight,
		NewBinary:         result.NewBinary,
		Duration:          result.Duration.String(),
		PreGenesisPath:    result.PreGenesisPath,
		PostGenesisPath:   result.PostGenesisPath,
	}

	data, err := json.MarshalIndent(jsonResult, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func outputUpgradeError(err error) error {
	errCode := "UPGRADE_FAILED"
	suggestion := ""

	result := map[string]interface{}{
		"error":   true,
		"code":    errCode,
		"message": err.Error(),
	}
	if suggestion != "" {
		result["suggestion"] = suggestion
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	return err
}

// buildBinaryForUpgrade builds a binary using DI container and BuildUseCase.
func buildBinaryForUpgrade(ctx context.Context, blockchainNetwork, ref, networkType, homeDir string, logger *output.Logger) (*dto.BuildOutput, error) {
	// Get network module
	networkModule, err := network.Get(blockchainNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to get network module: %w", err)
	}

	// Create DI factory with network module
	factory := di.NewInfrastructureFactory(homeDir, logger).
		WithNetworkModule(networkModule)

	// Wire container
	container, err := factory.WireContainer()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}

	logger.Info("Pre-building upgrade binary (ref: %s)...", ref)

	// Execute BuildUseCase
	return container.BuildUseCase().Execute(ctx, dto.BuildInput{
		Ref:      ref,
		Network:  networkType,
		UseCache: true,
		ToCache:  true,
	})
}

// PortsMetadataAdapter wraps *ports.DevnetMetadata for compatibility.
type PortsMetadataAdapter struct {
	metadata      *ports.DevnetMetadata
	svc           *application.DevnetService
	networkModule network.NetworkModule
}

// NewPortsMetadataAdapter creates a new adapter wrapping the given ports metadata.
func NewPortsMetadataAdapter(m *ports.DevnetMetadata, svc *application.DevnetService, nm network.NetworkModule) *PortsMetadataAdapter {
	return &PortsMetadataAdapter{
		metadata:      m,
		svc:           svc,
		networkModule: nm,
	}
}

func (a *PortsMetadataAdapter) GetChainID() string {
	return a.metadata.ChainID
}

func (a *PortsMetadataAdapter) GetExecutionMode() types.ExecutionMode {
	return a.metadata.ExecutionMode
}

func (a *PortsMetadataAdapter) SetExecutionMode(mode types.ExecutionMode) {
	a.metadata.ExecutionMode = mode
}

func (a *PortsMetadataAdapter) GetVersion() string {
	return a.metadata.CurrentVersion
}

func (a *PortsMetadataAdapter) SetVersion(version string) {
	a.metadata.CurrentVersion = version
}

func (a *PortsMetadataAdapter) GetNumValidators() int {
	return a.metadata.NumValidators
}

func (a *PortsMetadataAdapter) GetBinaryName() string {
	return a.networkModule.BinaryName()
}

func (a *PortsMetadataAdapter) GetHomeDir() string {
	return a.metadata.HomeDir
}

func (a *PortsMetadataAdapter) Save() error {
	ctx := context.Background()
	return a.svc.SaveMetadata(ctx, a.metadata)
}
