package cosmos

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// CosmosNetwork implements network.Module for Cosmos Hub.
type CosmosNetwork struct {
	runCmd commandRunnerFunc
	hooks  Hooks

	customizationFromFile   Customization
	customizationFromOption Customization
	customization           Customization

	profiles         map[string]networkProfile
	restToRPCHostMap map[string]string
	rpcToRESTHostMap map[string]string

	requestTimeout         time.Duration
	waitBlockTimeout       time.Duration
	snapshotResolveTimeout time.Duration

	initErr error
}

// Option configures CosmosNetwork construction.
type Option func(*CosmosNetwork)

var _ network.Module = (*CosmosNetwork)(nil)
var _ network.FileBasedGenesisModifier = (*CosmosNetwork)(nil)

// New creates a CosmosNetwork plugin module.
func New(opts ...Option) *CosmosNetwork {
	networkModule := &CosmosNetwork{
		runCmd: runCommand,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(networkModule)
		}
	}

	networkModule.applyConfiguration()

	return networkModule
}

// WithCommandRunner overrides command execution for this module instance.
func WithCommandRunner(runner func(context.Context, string, ...string) ([]byte, error)) Option {
	return func(n *CosmosNetwork) {
		if runner == nil {
			return
		}
		n.runCmd = runner
	}
}

// ============================================
// Identity
// ============================================

func (n *CosmosNetwork) Name() string {
	return "cosmos"
}

func (n *CosmosNetwork) DisplayName() string {
	return "Cosmos Hub"
}

func (n *CosmosNetwork) Version() string {
	return "1.2.0"
}

// ============================================
// Binary
// ============================================

func (n *CosmosNetwork) BinaryName() string {
	if v := strings.TrimSpace(n.customization.Runtime.BinaryName); v != "" {
		return v
	}
	return "gaiad"
}

func (n *CosmosNetwork) BinarySource() network.BinarySource {
	return network.BinarySource{
		Type:      "github",
		Owner:     "cosmos",
		Repo:      "gaia",
		AssetName: "gaiad-*-linux-amd64",
	}
}

func (n *CosmosNetwork) DefaultBinaryVersion() string {
	return "v25.3.2"
}

func (n *CosmosNetwork) GetBuildConfig(networkType string) (*network.BuildConfig, error) {
	return &network.BuildConfig{}, nil
}

// ============================================
// Chain config
// ============================================

func (n *CosmosNetwork) DefaultChainID() string {
	return "cosmosdevnet-1"
}

func (n *CosmosNetwork) Bech32Prefix() string {
	return "cosmos"
}

func (n *CosmosNetwork) BaseDenom() string {
	return "uatom"
}

func (n *CosmosNetwork) GenesisConfig() network.GenesisConfig {
	return network.GenesisConfig{
		ChainIDPattern:    "cosmosdevnet-{num}",
		EVMChainID:        0,
		BaseDenom:         "uatom",
		DenomExponent:     6,
		DisplayDenom:      "ATOM",
		BondDenom:         "uatom",
		MinSelfDelegation: "1",
		UnbondingTime:     120 * time.Second,
		MaxValidators:     100,
		MinDeposit:        "10000000uatom",
		VotingPeriod:      60 * time.Second,
		MaxDepositPeriod:  120 * time.Second,
		CommunityTax:      "0.020000000000000000",
	}
}

func (n *CosmosNetwork) DefaultPorts() network.PortConfig {
	return network.PortConfig{
		RPC:       26657,
		P2P:       26656,
		GRPC:      9090,
		GRPCWeb:   9091,
		API:       1317,
		EVMRPC:    0,
		EVMSocket: 0,
	}
}

// ============================================
// Docker
// ============================================

func (n *CosmosNetwork) DockerImage() string {
	if v := strings.TrimSpace(n.customization.Runtime.DockerImage); v != "" {
		return v
	}
	return "ghcr.io/cosmos/gaia:" + n.DefaultBinaryVersion()
}

func (n *CosmosNetwork) DockerImageTag(version string) string {
	if version == "" {
		return n.DefaultBinaryVersion()
	}
	return version
}

func (n *CosmosNetwork) DockerHomeDir() string {
	if v := strings.TrimSpace(n.customization.Runtime.DockerHomeDir); v != "" {
		return v
	}
	return "/home/gaia"
}

// ============================================
// Path config
// ============================================

func (n *CosmosNetwork) DefaultNodeHome() string {
	if v := strings.TrimSpace(n.customization.Runtime.DefaultHome); v != "" {
		return v
	}
	return "/root/.gaia"
}

func (n *CosmosNetwork) PIDFileName() string {
	return "gaiad.pid"
}

func (n *CosmosNetwork) LogFileName() string {
	return "gaiad.log"
}

func (n *CosmosNetwork) ProcessPattern() string {
	return "gaiad.*start"
}

// ============================================
// Commands
// ============================================

func (n *CosmosNetwork) InitCommand(homeDir, chainID, moniker string) []string {
	return []string{"init", moniker, "--chain-id", chainID, "--home", homeDir}
}

func (n *CosmosNetwork) StartCommand(homeDir string, networkMode string) []string {
	// Disable fastnode migration by default for forked large-state devnets.
	// This avoids known startup failures during IAVL fastnode upgrade on exported state.
	// Force pebbledb for large forked-state startup stability (goleveldb+snappy can panic on huge states).
	return []string{"start", "--home", homeDir, "--iavl-disable-fastnode", "--db_backend=pebbledb"}
}

func (n *CosmosNetwork) ExportCommand(homeDir string) []string {
	return []string{"export", "--home", homeDir}
}

func (n *CosmosNetwork) DefaultGeneratorConfig() network.GeneratorConfig {
	validatorBalance := strings.TrimSpace(n.customization.Funding.ValidatorBalance)
	if validatorBalance == "" {
		validatorBalance = "1000000000000uatom"
	}
	accountBalance := strings.TrimSpace(n.customization.Funding.AccountBalance)
	if accountBalance == "" {
		accountBalance = "100000000000uatom"
	}
	validatorStake := strings.TrimSpace(n.customization.Funding.ValidatorStakeDefault)
	if validatorStake == "" {
		validatorStake = "100000000"
	}

	return network.GeneratorConfig{
		NumValidators:    4,
		NumAccounts:      10,
		AccountBalance:   accountBalance,
		ValidatorBalance: validatorBalance,
		ValidatorStake:   validatorStake,
		OutputDir:        "./devnet",
		ChainID:          "cosmosdevnet-1",
	}
}

// ============================================
// Codec / validation
// ============================================

func (n *CosmosNetwork) GetCodec() ([]byte, error) {
	return []byte{}, nil
}

func (n *CosmosNetwork) Validate() error {
	if n.initErr != nil {
		return n.initErr
	}
	if n.Name() == "" {
		return fmt.Errorf("network name is required")
	}
	if n.BinaryName() == "" {
		return fmt.Errorf("binary name is required")
	}
	return nil
}

// ============================================
// Snapshot / RPC endpoints
// ============================================

func (n *CosmosNetwork) SnapshotURL(networkType string) string {
	profile, err := n.requireNetworkProfile(networkType)
	if err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.snapshotResolveTimeout)
	defer cancel()

	profileCfg := NetworkProfileConfig{
		ChainID:          profile.ChainID,
		RPCEndpoint:      profile.RPCEndpoint,
		RESTEndpoint:     profile.RESTEndpoint,
		SnapshotIndexURL: profile.SnapshotIndexURL,
	}

	var snapshotURL string
	if n.hooks.SnapshotURLResolver != nil {
		snapshotURL, err = n.hooks.SnapshotURLResolver(ctx, canonicalNetworkType(networkType), profileCfg, n.customization)
	} else {
		snapshotURL, err = resolveLatestPolkachuSnapshotURL(ctx, canonicalNetworkType(networkType), profileCfg, n.customization.Snapshot)
	}
	if err != nil {
		return ""
	}

	return strings.TrimSpace(snapshotURL)
}

func (n *CosmosNetwork) RPCEndpoint(networkType string) string {
	if profile, ok := n.networkProfileByType(networkType); ok {
		return profile.RPCEndpoint
	}
	return ""
}

func (n *CosmosNetwork) SnapshotURLs(networkType string) []string {
	snapshotURL := n.SnapshotURL(networkType)
	if snapshotURL == "" {
		return nil
	}
	return []string{snapshotURL}
}

func (n *CosmosNetwork) RPCEndpoints(networkType string) []string {
	endpoint := n.RPCEndpoint(networkType)
	if endpoint == "" {
		return nil
	}
	return []string{endpoint}
}

func (n *CosmosNetwork) AvailableNetworks() []string {
	return availableNetworkTypes(n.profiles)
}

func availableNetworkTypes(profiles map[string]networkProfile) []string {
	if len(profiles) == 0 {
		return []string{networkMainnet, networkTestnet}
	}

	out := make([]string, 0, len(profiles))
	if _, ok := profiles[networkMainnet]; ok {
		out = append(out, networkMainnet)
	}
	if _, ok := profiles[networkTestnet]; ok {
		out = append(out, networkTestnet)
	}

	extras := make([]string, 0, len(profiles))
	for key := range profiles {
		if key == networkMainnet || key == networkTestnet {
			continue
		}
		extras = append(extras, key)
	}
	sort.Strings(extras)
	out = append(out, extras...)

	return out
}
