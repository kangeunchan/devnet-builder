package cosmos

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// CosmosNetwork implements network.Module for Cosmos Hub.
type CosmosNetwork struct {
	runCmd           commandRunnerFunc
	snapshotResolver snapshotResolverFunc
}

// Option configures CosmosNetwork construction.
type Option func(*CosmosNetwork)

var _ network.Module = (*CosmosNetwork)(nil)
var _ network.FileBasedGenesisModifier = (*CosmosNetwork)(nil)

// New creates a CosmosNetwork plugin module.
func New(opts ...Option) *CosmosNetwork {
	networkModule := &CosmosNetwork{
		runCmd:           runCommand,
		snapshotResolver: resolveLatestPolkachuSnapshotURL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(networkModule)
		}
	}

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

// WithSnapshotResolver overrides snapshot URL resolution for this module instance.
func WithSnapshotResolver(resolver func(networkType string) string) Option {
	return func(n *CosmosNetwork) {
		if resolver == nil {
			return
		}
		n.snapshotResolver = func(ctx context.Context, networkType string) (string, error) {
			return resolver(networkType), nil
		}
	}
}

// WithSnapshotHTTPClient allows tests to control snapshot index fetch behavior
// without exposing internal resolver helpers.
func WithSnapshotHTTPClient(client *http.Client) Option {
	return func(n *CosmosNetwork) {
		n.snapshotResolver = func(ctx context.Context, networkType string) (string, error) {
			return resolveLatestPolkachuSnapshotURLWithClient(ctx, networkType, client)
		}
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
	return "1.1.0"
}

// ============================================
// Binary
// ============================================

func (n *CosmosNetwork) BinaryName() string {
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
	return "ghcr.io/cosmos/gaia:" + n.DefaultBinaryVersion()
}

func (n *CosmosNetwork) DockerImageTag(version string) string {
	if version == "" {
		return n.DefaultBinaryVersion()
	}
	return version
}

func (n *CosmosNetwork) DockerHomeDir() string {
	return "/home/gaia"
}

// ============================================
// Path config
// ============================================

func (n *CosmosNetwork) DefaultNodeHome() string {
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
	args := []string{"start", "--home", homeDir}
	if profile, ok := networkProfileByType(networkMode); ok {
		args = append(args, "--chain-id", profile.ChainID)
	}
	return args
}

func (n *CosmosNetwork) ExportCommand(homeDir string) []string {
	return []string{"export", "--home", homeDir}
}

func (n *CosmosNetwork) DefaultGeneratorConfig() network.GeneratorConfig {
	return network.GeneratorConfig{
		NumValidators:    4,
		NumAccounts:      10,
		AccountBalance:   "100000000000uatom",
		ValidatorBalance: "1000000000000uatom",
		ValidatorStake:   "100000000",
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
	if n.snapshotResolver == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), snapshotResolverTimeout)
	defer cancel()

	snapshotURL, err := n.snapshotResolver(ctx, networkType)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(snapshotURL)
}

func (n *CosmosNetwork) RPCEndpoint(networkType string) string {
	if profile, ok := networkProfileByType(networkType); ok {
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
	return []string{networkMainnet, networkTestnet}
}
