package manage

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application"
	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/config"
	"github.com/altuslabsxyz/devnet-builder/internal/di"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/binary"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/cache"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/executor"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/filesystem"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/github"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/interactive"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/network"
	"github.com/altuslabsxyz/devnet-builder/internal/output"
	"github.com/altuslabsxyz/devnet-builder/internal/paths"
	"github.com/altuslabsxyz/devnet-builder/types"
	"github.com/altuslabsxyz/devnet-builder/types/ctxconfig"
	"github.com/spf13/cobra"
)

var (
	deployNetwork           string
	deployBlockchainNetwork string // Network module selection (stable, ault, etc.)
	deployValidators        int
	deployMode              string
	deployStableVersion     string
	deployNoCache           bool
	deployAccounts          int
	deployNoInteractive     bool
	deployStartVersion      string
	deployExportVersion     string // Version for export binary (genesis export), optional
	deployImage             string
	deployFork              bool   // Fork live network state via snapshot export
	deployTestMnemonic      bool   // Use deterministic test mnemonics for validators
	deployBinary            string // Custom binary path for local mode
)

// DeployResult represents the JSON output for the deploy command.
type DeployResult struct {
	Status            string             `json:"status"`
	ChainID           string             `json:"chain_id"`
	Network           string             `json:"network"`            // Snapshot source: mainnet/testnet
	BlockchainNetwork string             `json:"blockchain_network"` // Network module: stable/ault
	Mode              string             `json:"mode"`
	DockerImage       string             `json:"docker_image,omitempty"`
	Validators        int                `json:"validators"`
	Nodes             []types.NodeResult `json:"nodes"`
}

// NewDeployCmd creates the deploy command.
func NewDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a local devnet (provision + start)",
		Long: `Deploy a local devnet with configurable validators and network source.

This command will:
1. Check prerequisites (Docker/local binary, curl, jq, zstd/lz4)
2. Download or use cached snapshot from the specified network
3. Export genesis state from snapshot
4. Generate devnet configuration for all validators
5. Start all validator nodes

Docker mode supports 1-100 validators with network isolation and automatic port management.
Local mode supports 1-4 validators for testing and development.

Examples:
  # Deploy with default settings (4 validators, mainnet, docker mode)
  devnet-builder deploy

  # Deploy with testnet data
  devnet-builder deploy --network testnet

  # Deploy with 10 validators in docker mode (isolated network, auto port allocation)
  devnet-builder deploy --validators 10

  # Deploy with 2 validators in local binary mode
  devnet-builder deploy --mode local --validators 2

  # Deploy with specific stable version
  devnet-builder deploy --network-version v1.2.3

  # Large-scale deployment with 100 validators (docker mode only)
  devnet-builder deploy --validators 100

  # Deploy with custom binary in local mode (interactive selection)
  devnet-builder deploy --mode local
  → Then select "Use local binary" and browse to your binary

  # Deploy with fork mode (binary needed for genesis export)
  devnet-builder deploy --mode local --fork

  # Deploy with different binary versions for export and start
  # (useful when export requires a specific version for state compatibility)
  devnet-builder deploy --mode local --start-version v1.2.3 --export-version v1.1.0`,
		RunE: runDeploy,
	}

	// Command flags
	cmd.Flags().StringVarP(&deployNetwork, "network", "n", "mainnet",
		"Network source (mainnet, testnet)")
	cmd.Flags().IntVar(&deployValidators, "validators", 4,
		"Number of validators (1-100 for docker mode, 1-4 for local mode)")
	cmd.Flags().StringVarP(&deployMode, "mode", "m", "docker",
		"Execution mode (docker, local)")
	cmd.Flags().StringVar(&deployStableVersion, "network-version", "latest",
		"Network repository version")
	cmd.Flags().BoolVar(&deployNoCache, "no-cache", false,
		"Skip snapshot cache")
	cmd.Flags().IntVar(&deployAccounts, "accounts", 4,
		"Additional funded accounts")
	cmd.Flags().BoolVar(&deployTestMnemonic, "test-mnemonic", true,
		"Use deterministic test mnemonics for validators (disable for production-like testing)")

	// Interactive mode flags (controls version/docker image selection prompts)
	// Note: Base config prompts (network, validators, mode) are handled by config.toml
	cmd.Flags().BoolVar(&deployNoInteractive, "no-interactive", false,
		"Disable version selection prompts (use --start-version, --image instead)")
	cmd.Flags().StringVar(&deployStartVersion, "start-version", "",
		"Version for devnet binary (non-interactive mode)")
	cmd.Flags().StringVar(&deployExportVersion, "export-version", "",
		"Version for genesis export binary (optional, defaults to start-version)")

	// Docker image flag
	cmd.Flags().StringVar(&deployImage, "image", "",
		"Docker image for docker mode (e.g., v1.0.0 or ghcr.io/org/image:tag)")

	// Blockchain network module flag
	cmd.Flags().StringVar(&deployBlockchainNetwork, "blockchain", "stable",
		"Blockchain network module (stable, ault)")

	// Fork mode flag - exports genesis from snapshot state instead of RPC genesis
	cmd.Flags().BoolVar(&deployFork, "fork", true,
		"Fork live network state (export genesis from snapshot)")

	return cmd
}

type deployContext struct {
	cmd      *cobra.Command
	ctx      context.Context
	homeDir  string
	jsonMode bool
	logger   *output.Logger
	fileCfg  *config.FileConfig
}

type deployResolvedConfig struct {
	fileCfg           *config.FileConfig
	network           string
	blockchainNetwork string
	validators        int
	mode              string
	stableVersion     string
	noCache           bool
	accounts          int
	testMnemonic      bool
	fork              bool
	isInteractive     bool
}

type deployPreparedInputs struct {
	startVersion     string
	dockerImage      string
	customBinaryPath string
	exportBinaryPath string
	networkModule    network.NetworkModule
	svc              *application.DevnetService
}

type deployExecutionResult struct {
	runResult  *dto.RunOutput
	devnetInfo *dto.DevnetInfo
}

func runDeploy(cmd *cobra.Command, args []string) error {
	deployCtx := newDeployContext(cmd)

	resolvedConfig, err := resolveDeployConfig(deployCtx)
	if err != nil {
		return err
	}

	deployCtx.logger.SetAutoSpinner(true)
	defer deployCtx.logger.SetAutoSpinner(false)

	preparedInputs, err := prepareDeployInputs(deployCtx, resolvedConfig)
	if err != nil {
		return err
	}

	executionResult, err := executeDeployment(deployCtx, resolvedConfig, preparedInputs)
	if err != nil {
		if deployCtx.jsonMode {
			return outputDeployError(err)
		}
		return err
	}

	if deployCtx.jsonMode {
		return outputDeployJSON(executionResult.runResult, executionResult.devnetInfo)
	}
	return outputDeployText(executionResult.runResult, executionResult.devnetInfo)
}

func newDeployContext(cmd *cobra.Command) *deployContext {
	ctx := cmd.Context()
	cfg := ctxconfig.FromContext(ctx)
	fileCfg := cfg.FileConfig()
	if fileCfg == nil {
		fileCfg = &config.FileConfig{}
	}

	return &deployContext{
		cmd:      cmd,
		ctx:      ctx,
		homeDir:  cfg.HomeDir(),
		jsonMode: cfg.JSONMode(),
		logger:   output.DefaultLogger,
		fileCfg:  fileCfg,
	}
}

func resolveDeployConfig(deployCtx *deployContext) (*deployResolvedConfig, error) {
	applyDeployFlagOverrides(deployCtx.cmd, deployCtx.fileCfg)
	applyDeployEnvOverrides(deployCtx.cmd, deployCtx.fileCfg)

	setup := config.NewInteractiveSetup(deployCtx.homeDir)
	effectiveCfg, err := setup.RunPartial(deployCtx.fileCfg)
	if err != nil {
		if mfErr, ok := err.(*config.MissingFieldsError); ok {
			return nil, fmt.Errorf("missing required configuration: %v\nRun 'devnet-builder config init' to create a configuration file", mfErr.Fields)
		}
		return nil, err
	}

	resolvedConfig := extractDeployResolvedConfig(effectiveCfg, deployCtx.jsonMode)
	if err := validateDeployConfiguration(resolvedConfig); err != nil {
		return nil, err
	}

	return resolvedConfig, nil
}

func applyDeployFlagOverrides(cmd *cobra.Command, fileCfg *config.FileConfig) {
	if cmd.Flags().Changed("network") {
		fileCfg.Network = &deployNetwork
	}
	if cmd.Flags().Changed("blockchain") {
		fileCfg.BlockchainNetwork = &deployBlockchainNetwork
	}
	if cmd.Flags().Changed("validators") {
		fileCfg.Validators = &deployValidators
	}
	if cmd.Flags().Changed("mode") {
		mode := types.ExecutionMode(deployMode)
		fileCfg.ExecutionMode = &mode
	}
	if cmd.Flags().Changed("network-version") {
		fileCfg.NetworkVersion = &deployStableVersion
	}
	if cmd.Flags().Changed("no-cache") {
		fileCfg.NoCache = &deployNoCache
	}
	if cmd.Flags().Changed("accounts") {
		fileCfg.Accounts = &deployAccounts
	}
}

func applyDeployEnvOverrides(cmd *cobra.Command, fileCfg *config.FileConfig) {
	if networkEnv := os.Getenv("DEVNET_NETWORK"); networkEnv != "" && !cmd.Flags().Changed("network") {
		fileCfg.Network = &networkEnv
	}
	if modeEnv := os.Getenv("DEVNET_MODE"); modeEnv != "" && !cmd.Flags().Changed("mode") {
		mode := types.ExecutionMode(modeEnv)
		fileCfg.ExecutionMode = &mode
	}
	if versionEnv := os.Getenv("DEVNET_NETWORK_VERSION"); versionEnv != "" && !cmd.Flags().Changed("network-version") {
		fileCfg.NetworkVersion = &versionEnv
	}
}

func extractDeployResolvedConfig(effectiveCfg *config.FileConfig, jsonMode bool) *deployResolvedConfig {
	deployNetwork = *effectiveCfg.Network
	deployBlockchainNetwork = *effectiveCfg.BlockchainNetwork
	deployValidators = *effectiveCfg.Validators
	deployMode = string(*effectiveCfg.ExecutionMode)
	if effectiveCfg.NetworkVersion != nil {
		deployStableVersion = *effectiveCfg.NetworkVersion
	}
	if effectiveCfg.NoCache != nil {
		deployNoCache = *effectiveCfg.NoCache
	}
	if effectiveCfg.Accounts != nil {
		deployAccounts = *effectiveCfg.Accounts
	}

	return &deployResolvedConfig{
		fileCfg:           effectiveCfg,
		network:           deployNetwork,
		blockchainNetwork: deployBlockchainNetwork,
		validators:        deployValidators,
		mode:              deployMode,
		stableVersion:     deployStableVersion,
		noCache:           deployNoCache,
		accounts:          deployAccounts,
		testMnemonic:      deployTestMnemonic,
		fork:              deployFork,
		isInteractive:     shouldRunDeployInteractiveSelection(jsonMode),
	}
}

func validateDeployConfiguration(resolvedConfig *deployResolvedConfig) error {
	if !types.NetworkSource(resolvedConfig.network).IsValid() {
		return fmt.Errorf("invalid network: %s (must be 'mainnet' or 'testnet')", resolvedConfig.network)
	}

	if resolvedConfig.mode == string(types.ExecutionModeDocker) {
		if resolvedConfig.validators < 1 || resolvedConfig.validators > 100 {
			return fmt.Errorf("invalid validators: %d (must be 1-100 for docker mode)", resolvedConfig.validators)
		}
	} else if resolvedConfig.mode == string(types.ExecutionModeLocal) {
		if resolvedConfig.validators < 1 || resolvedConfig.validators > 4 {
			return fmt.Errorf("invalid validators: %d (must be 1-4 for local mode)", resolvedConfig.validators)
		}
		if err := validateLocalModePorts(resolvedConfig.validators); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("invalid mode: %s (must be 'docker' or 'local')", resolvedConfig.mode)
	}

	return nil
}

func shouldRunDeployInteractiveSelection(jsonMode bool) bool {
	return !deployNoInteractive && !jsonMode && deployBinary == ""
}

func prepareDeployInputs(deployCtx *deployContext, resolvedConfig *deployResolvedConfig) (*deployPreparedInputs, error) {
	preparedInputs := &deployPreparedInputs{}

	if resolvedConfig.mode == string(types.ExecutionModeDocker) {
		resolvedImage, err := resolveDeployDockerImage(
			deployCtx.ctx,
			deployCtx.cmd,
			resolvedConfig.isInteractive,
			deployCtx.homeDir,
			resolvedConfig.fileCfg,
		)
		if err != nil {
			return nil, WrapInteractiveError(deployCtx.cmd, err, "failed to resolve docker image")
		}
		preparedInputs.dockerImage = resolvedImage
		preparedInputs.startVersion = resolvedConfig.stableVersion
	} else {
		startVersion, customBinaryPath, err := resolveDeployLocalVersionSelection(deployCtx, resolvedConfig)
		if err != nil {
			return nil, err
		}
		preparedInputs.startVersion = startVersion
		preparedInputs.customBinaryPath = customBinaryPath
	}

	if err := validateDeprecatedDeployBinaryFlag(); err != nil {
		return nil, err
	}

	networkModule, err := resolveDeployNetworkModule(resolvedConfig.blockchainNetwork)
	if err != nil {
		return nil, err
	}
	preparedInputs.networkModule = networkModule

	svc, err := application.GetServiceWithConfig(application.ServiceConfig{
		HomeDir:       deployCtx.homeDir,
		NetworkModule: networkModule,
		DockerMode:    resolvedConfig.mode == string(types.ExecutionModeDocker),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize service: %w", err)
	}
	if svc.DevnetExists() {
		return nil, fmt.Errorf("devnet already exists at %s\nUse 'devnet-builder destroy' to remove it first", deployCtx.homeDir)
	}
	preparedInputs.svc = svc

	if resolvedConfig.mode == string(types.ExecutionModeLocal) {
		customBinaryPath, exportBinaryPath, err := resolveLocalDeployBinaryPaths(deployCtx, resolvedConfig, preparedInputs)
		if err != nil {
			return nil, err
		}
		preparedInputs.customBinaryPath = customBinaryPath
		preparedInputs.exportBinaryPath = exportBinaryPath
	}

	return preparedInputs, nil
}

func resolveDeployLocalVersionSelection(
	deployCtx *deployContext,
	resolvedConfig *deployResolvedConfig,
) (string, string, error) {
	startVersion := ""
	customBinaryPath := ""

	if resolvedConfig.isInteractive {
		selection, err := RunInteractiveVersionSelection(
			deployCtx.ctx,
			deployCtx.cmd,
			false,
			resolvedConfig.network,
			resolvedConfig.blockchainNetwork,
		)
		if err != nil {
			return "", "", WrapInteractiveError(deployCtx.cmd, err, "failed during interactive selection")
		}

		startVersion = selection.StartVersion
		deployStableVersion = startVersion

		if selection.BinarySource != nil && selection.BinarySource.IsLocal() && selection.BinarySource.SelectedPath != "" {
			customBinaryPath = selection.BinarySource.SelectedPath
		} else if selection.BinarySource != nil && selection.BinarySource.IsGitHubRelease() && startVersion != "" {
			buildResult, err := buildBinaryForDeploy(
				deployCtx.ctx,
				resolvedConfig.blockchainNetwork,
				startVersion,
				resolvedConfig.network,
				deployCtx.homeDir,
				deployCtx.logger,
			)
			if err != nil {
				return "", "", fmt.Errorf("failed to pre-build binary: %w", err)
			}
			customBinaryPath = buildResult.BinaryPath
			commitShort := buildResult.CommitHash
			if len(commitShort) > 12 {
				commitShort = commitShort[:12]
			}
			deployCtx.logger.Success("Binary pre-built and cached (commit: %s)", commitShort)
		}
	} else {
		if deployStartVersion != "" {
			startVersion = deployStartVersion
		} else {
			startVersion = resolvedConfig.stableVersion
		}
	}

	return startVersion, customBinaryPath, nil
}

func validateDeprecatedDeployBinaryFlag() error {
	if deployBinary == "" {
		return nil
	}

	return fmt.Errorf(`the --binary flag has been removed in favor of interactive binary selection

When you run 'devnet-builder deploy' in interactive mode, you will be prompted to:
1. Choose between using a local binary or downloading from GitHub releases
2. If you select "local binary", you can browse your filesystem with Tab autocomplete

Migration guide:
  • Interactive mode (recommended):
      devnet-builder deploy
      → Select "Use local binary (browse filesystem)"
      → Navigate to your binary using Tab autocomplete

  • Non-interactive mode with environment variable:
      export DEVNET_BINARY_PATH=/path/to/your/binary
      devnet-builder deploy --no-interactive

  • Docker mode (unchanged):
      devnet-builder deploy --mode docker --image your-image:tag

For more information, see: https://github.com/altuslabsxyz/devnet-builder/blob/main/docs/MIGRATION.md`)
}

func resolveDeployNetworkModule(blockchainNetwork string) (network.NetworkModule, error) {
	if !network.Has(blockchainNetwork) {
		available := network.List()
		return nil, fmt.Errorf("unknown blockchain network: %s (available: %v)", blockchainNetwork, available)
	}

	networkModule, err := network.Get(blockchainNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to get network module: %w", err)
	}

	return networkModule, nil
}

func resolveLocalDeployBinaryPaths(
	deployCtx *deployContext,
	resolvedConfig *deployResolvedConfig,
	preparedInputs *deployPreparedInputs,
) (string, string, error) {
	customBinaryPath := preparedInputs.customBinaryPath

	if customBinaryPath == "" {
		selectedPath, err := selectBinaryForDeployment(
			deployCtx.ctx,
			resolvedConfig.network,
			resolvedConfig.blockchainNetwork,
			deployCtx.homeDir,
			deployCtx.logger,
		)
		if err != nil {
			return "", "", fmt.Errorf("binary selection failed: %w", err)
		}

		if selectedPath == "" {
			buildResult, err := buildBinaryForDeploy(
				deployCtx.ctx,
				resolvedConfig.blockchainNetwork,
				preparedInputs.startVersion,
				resolvedConfig.network,
				deployCtx.homeDir,
				deployCtx.logger,
			)
			if err != nil {
				return "", "", fmt.Errorf("failed to build from source: %w", err)
			}
			customBinaryPath = buildResult.BinaryPath
			deployCtx.logger.Success("Binary built: %s (commit: %s)", buildResult.BinaryPath, buildResult.CommitHash)
		} else {
			customBinaryPath = selectedPath
		}
	} else {
		deployCtx.logger.Success("Using selected binary: %s", customBinaryPath)
	}

	exportBinaryPath := customBinaryPath
	if deployExportVersion != "" {
		deployCtx.logger.Info("Building export binary version: %s", deployExportVersion)
		buildResult, err := buildBinaryForDeploy(
			deployCtx.ctx,
			resolvedConfig.blockchainNetwork,
			deployExportVersion,
			resolvedConfig.network,
			deployCtx.homeDir,
			deployCtx.logger,
		)
		if err != nil {
			return "", "", fmt.Errorf("failed to build export binary: %w", err)
		}
		exportBinaryPath = buildResult.BinaryPath
		commitShort := buildResult.CommitHash
		if len(commitShort) > 12 {
			commitShort = commitShort[:12]
		}
		deployCtx.logger.Success("Export binary ready (version: %s, commit: %s)", deployExportVersion, commitShort)
	}

	return customBinaryPath, exportBinaryPath, nil
}

func executeDeployment(
	deployCtx *deployContext,
	resolvedConfig *deployResolvedConfig,
	preparedInputs *deployPreparedInputs,
) (*deployExecutionResult, error) {
	if resolvedConfig.mode == string(types.ExecutionModeDocker) {
		return executeDockerDeployment(deployCtx, resolvedConfig, preparedInputs)
	}
	return executeLocalDeployment(deployCtx, resolvedConfig, preparedInputs)
}

func executeDockerDeployment(
	deployCtx *deployContext,
	resolvedConfig *deployResolvedConfig,
	preparedInputs *deployPreparedInputs,
) (*deployExecutionResult, error) {
	return runDeployProvisionAndStart(deployCtx, resolvedConfig, preparedInputs)
}

func executeLocalDeployment(
	deployCtx *deployContext,
	resolvedConfig *deployResolvedConfig,
	preparedInputs *deployPreparedInputs,
) (*deployExecutionResult, error) {
	return runDeployProvisionAndStart(deployCtx, resolvedConfig, preparedInputs)
}

func runDeployProvisionAndStart(
	deployCtx *deployContext,
	resolvedConfig *deployResolvedConfig,
	preparedInputs *deployPreparedInputs,
) (*deployExecutionResult, error) {
	provisionInput := dto.ProvisionInput{
		HomeDir:           deployCtx.homeDir,
		Network:           resolvedConfig.network,
		BlockchainNetwork: resolvedConfig.blockchainNetwork,
		NumValidators:     resolvedConfig.validators,
		NumAccounts:       resolvedConfig.accounts,
		Mode:              resolvedConfig.mode,
		StableVersion:     preparedInputs.startVersion,
		DockerImage:       preparedInputs.dockerImage,
		NoCache:           resolvedConfig.noCache,
		CustomBinaryPath:  preparedInputs.customBinaryPath,
		UseSnapshot:       resolvedConfig.fork,
		BinaryPath:        preparedInputs.exportBinaryPath,
		UseTestMnemonic:   resolvedConfig.testMnemonic,
	}

	if _, err := preparedInputs.svc.Provision(deployCtx.ctx, provisionInput); err != nil {
		return nil, err
	}

	runResult, err := preparedInputs.svc.Start(deployCtx.ctx, 5*time.Minute)
	if err != nil {
		return nil, err
	}

	devnetInfo, _ := preparedInputs.svc.LoadDevnetInfo(deployCtx.ctx)
	return &deployExecutionResult{
		runResult:  runResult,
		devnetInfo: devnetInfo,
	}, nil
}

func outputDeployText(result *dto.RunOutput, devnetInfo *dto.DevnetInfo) error {
	fmt.Println()
	output.Bold("Chain ID:     %s", devnetInfo.ChainID)
	output.Info("Network:      %s", devnetInfo.NetworkSource)
	output.Info("Blockchain:   %s", devnetInfo.BlockchainNetwork)
	output.Info("ExecutionMode:         %s", devnetInfo.ExecutionMode)
	if devnetInfo.DockerImage != "" {
		output.Info("Docker Image: %s", devnetInfo.DockerImage)
	}
	output.Info("Validators:   %d", devnetInfo.NumValidators)
	fmt.Println()
	output.Bold("Endpoints:")

	for i := range devnetInfo.Nodes {
		n := &devnetInfo.Nodes[i]
		status := "running"
		for _, ns := range result.Nodes {
			if ns.Index == n.Index && !ns.IsRunning {
				status = "failed"
				break
			}
		}
		if status == "running" {
			fmt.Printf("  Node %d: %s (RPC) | %s (EVM)\n",
				n.Index, n.RPCURL, n.EVMURL)
		} else {
			fmt.Printf("  Node %d: [FAILED]\n", n.Index)
		}
	}

	return nil
}

func outputDeployJSON(result *dto.RunOutput, devnetInfo *dto.DevnetInfo) error {
	jsonResult := DeployResult{
		Status:            "success",
		ChainID:           devnetInfo.ChainID,
		Network:           devnetInfo.NetworkSource,
		BlockchainNetwork: devnetInfo.BlockchainNetwork,
		Mode:              string(devnetInfo.ExecutionMode),
		DockerImage:       devnetInfo.DockerImage,
		Validators:        devnetInfo.NumValidators,
		Nodes:             make([]types.NodeResult, len(devnetInfo.Nodes)),
	}

	if !result.AllRunning {
		jsonResult.Status = "partial"
	}

	for i := range devnetInfo.Nodes {
		n := &devnetInfo.Nodes[i]
		status := "running"
		for _, ns := range result.Nodes {
			if ns.Index == n.Index && !ns.IsRunning {
				status = "failed"
				break
			}
		}
		jsonResult.Nodes[i] = types.NodeResult{
			Index:  n.Index,
			RPC:    n.RPCURL,
			EVMRPC: n.EVMURL,
			Status: status,
		}
	}

	data, err := json.MarshalIndent(jsonResult, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func outputDeployError(err error) error {
	result := map[string]interface{}{
		"error":   true,
		"code":    types.GetErrorCode(err),
		"message": err.Error(),
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	return err
}

// selectBinaryForDeployment orchestrates binary selection from cache or build.
// This implements US1 (Interactive Local Binary Selection) by scanning cache,
// validating binaries, and presenting an interactive selection to the user.
//
// Priority Order (FR-005):
//  1. Interactive/auto selection from cache (NEW)
//  2. Build from source (if selected or no cache)
func selectBinaryForDeployment(
	ctx context.Context,
	networkName string,
	blockchain string,
	homeDir string,
	logger *output.Logger,
) (string, error) {
	// Setup components
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
	// This handles edge cases where NetworkName might be empty or mismatched
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

	// Validate binaries concurrently (EC-006: 5s timeout per binary)
	validBinaries := filterValidBinaries(ctx, scannedBinaries, validator, logger)

	// If no valid binaries found, log and proceed to build
	// EC-001: Empty cache → caller will build from source
	if len(validBinaries) == 0 && len(scannedBinaries) > 0 {
		logger.Info("No valid cached binaries found, building from source")
	}

	// Run selection with appropriate options
	opts := interactive.BinarySelectionOptions{
		AllowBuildFromSource: true,                                // FR-011: Allow build option
		AutoSelectSingle:     true,                                // CLARIFICATION 1: Auto-select single binary
		IsInteractive:        interactive.IsTerminalInteractive(), // EC-004: TTY detection
	}

	result, err := selector.RunBinarySelectionFlow(ctx, validBinaries, opts)
	if err != nil {
		return "", fmt.Errorf("binary selection failed: %w", err)
	}

	// EC-005: User cancelled
	if result.WasCancelled {
		return "", fmt.Errorf("selection cancelled by user")
	}

	// User selected "Build from source" or no binaries available
	if result.ShouldBuild || result.SelectedBinary == nil {
		buildVersion := result.BuildVersion
		if buildVersion == "" {
			// No binaries and didn't explicitly select build → use default behavior
			// This handles EC-001: Empty cache scenario
			return "", nil // Caller will trigger default build
		}

		// User explicitly requested build with specific version
		logger.Info("Building binary from source: %s", buildVersion)
		buildResult, err := buildBinaryForDeploy(ctx, blockchain, buildVersion, networkName, homeDir, logger)
		if err != nil {
			return "", fmt.Errorf("failed to build from source: %w", err)
		}
		return buildResult.BinaryPath, nil
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

// filterValidBinaries validates binaries concurrently and returns only valid ones.
// This implements concurrent validation per spec requirement (Performance: ≤2.5s for 20 binaries).
func filterValidBinaries(
	ctx context.Context,
	binaries []cache.CachedBinaryMetadata,
	validator *binary.BinaryValidator,
	logger *output.Logger,
) []cache.CachedBinaryMetadata {
	if len(binaries) == 0 {
		return []cache.CachedBinaryMetadata{}
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

	// Collect valid binaries
	var validBinaries []cache.CachedBinaryMetadata
	for result := range results {
		if result.err != nil {
			// EC-007: Corrupted/invalid binary - warn and skip
			logger.Warn("Skipping invalid binary %s: %v", result.binary.Path, result.err)
			continue
		}
		if result.binary.IsValid {
			validBinaries = append(validBinaries, result.binary)
		}
	}

	// Sort by modification time descending (most recent first)
	// This maintains the order from scanner (CLARIFICATION 3)
	sort.Slice(validBinaries, func(i, j int) bool {
		return validBinaries[i].ModTime.After(validBinaries[j].ModTime)
	})

	return validBinaries
}

// resolveDeployDockerImage determines the docker image to use.
func resolveDeployDockerImage(ctx context.Context, cmd *cobra.Command, isInteractive bool, homeDir string, fileCfg *config.FileConfig) (string, error) {
	// Priority 1: --image flag was explicitly provided
	if cmd.Flags().Changed("image") && deployImage != "" {
		return types.NormalizeImageURL(deployImage), nil
	}

	// Priority 2: Interactive mode - prompt user to select
	if isInteractive && deployMode == string(types.ExecutionModeDocker) {
		imageSelection, err := runDeployDockerImageSelection(ctx, homeDir, fileCfg)
		if err != nil {
			return "", err
		}
		if imageSelection.IsCustom {
			return imageSelection.ImageTag, nil
		}
		return fmt.Sprintf("%s:%s", types.DefaultGHCRImage, imageSelection.ImageTag), nil
	}

	// Priority 3: Non-interactive mode with --image flag
	if deployImage != "" {
		return types.NormalizeImageURL(deployImage), nil
	}

	return "", nil
}

// runDeployDockerImageSelection prompts the user to select a docker image version.
func runDeployDockerImageSelection(ctx context.Context, homeDir string, fileCfg *config.FileConfig) (*types.DockerImageSelectionResult, error) {
	cacheTTL := github.DefaultCacheTTL
	if fileCfg != nil && fileCfg.CacheTTL != nil {
		if ttl, err := time.ParseDuration(*fileCfg.CacheTTL); err == nil {
			cacheTTL = ttl
		}
	}
	cacheManager := github.NewCacheManager(homeDir, cacheTTL)

	clientOpts := []github.ClientOption{
		github.WithCache(cacheManager),
	}

	// Resolve GitHub token from keychain, environment, or config file
	if token, found := ResolveGitHubToken(fileCfg); found {
		clientOpts = append(clientOpts, github.WithToken(token))
	}

	client := github.NewClient(clientOpts...)

	versions, fromCache, err := client.GetImageVersionsWithCache(ctx, types.DefaultDockerPackage)
	if err != nil {
		if warning, ok := err.(*github.StaleDataWarning); ok {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", warning.Message)
		} else {
			return nil, fmt.Errorf("failed to fetch docker image versions: %w", err)
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no docker image versions available. Check your network connection or GitHub token")
	}

	if fromCache {
		fmt.Println("(Using cached docker image data)")
	}

	imageTag, isCustom, err := interactive.SelectDockerImage(versions)
	if err != nil {
		return nil, err
	}

	return &types.DockerImageSelectionResult{
		ImageTag:  imageTag,
		IsCustom:  isCustom,
		FromCache: fromCache,
	}, nil
}

// buildBinaryForDeploy builds a binary using DI container and BuildUseCase.
// This replaces direct usage of the legacy builder package.
func buildBinaryForDeploy(ctx context.Context, blockchainNetwork, ref, networkType, homeDir string, logger *output.Logger) (*dto.BuildOutput, error) {
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

	logger.Info("Building binary from source (ref: %s)...", ref)

	// Execute BuildUseCase
	// Note: ToCache=false means Build() is called which creates the symlink
	// at ~/.devnet-builder/bin/stabled, required for key creation and node init
	return container.BuildUseCase().Execute(ctx, dto.BuildInput{
		Ref:      ref,
		Network:  networkType,
		UseCache: true,  // Check cache first
		ToCache:  false, // Use Build() to create symlink at bin/stabled
	})
}

// validateBinaryPath checks if the provided binary path exists and is executable.
// Returns absolute path if valid, error otherwise.
func validateBinaryPath(binaryPath string) (string, error) {
	// Convert to absolute path
	absPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return "", fmt.Errorf("invalid binary path: %w", err)
	}

	// Check if file exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("binary not found at path: %s", absPath)
		}
		return "", fmt.Errorf("failed to check binary: %w", err)
	}

	// Check if it's a regular file (not a directory)
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a binary: %s", absPath)
	}

	// Check if executable (Unix permissions)
	mode := info.Mode()
	if mode&0111 == 0 {
		return "", fmt.Errorf("binary is not executable: %s (use chmod +x)", absPath)
	}

	return absPath, nil
}

// validateLocalModePorts checks if the ports needed for local mode deployment are available.
// Returns an error with conflicting ports if any are already in use.
func validateLocalModePorts(validatorCount int) error {
	var conflicts []int

	// Check ports for each validator
	for i := 0; i < validatorCount; i++ {
		portConfig := types.PortConfigForNode(i)
		for _, port := range portConfig.AllPorts() {
			if isPortInUse(port) {
				conflicts = append(conflicts, port)
			}
		}
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("port conflict detected: ports %v already in use", conflicts)
	}

	return nil
}

// isPortInUse checks if a TCP port is currently in use by attempting to listen on it.
func isPortInUse(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Port is in use or unavailable
		return true
	}
	listener.Close()
	return false
}
