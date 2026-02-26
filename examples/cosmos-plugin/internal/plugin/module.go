package cosmos

import (
	"context"
	"fmt"
	"time"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// CosmosNetwork implements network.Module for Cosmos Hub.
type CosmosNetwork struct {
	runCmd commandRunnerFunc
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
	return "v18.1.0"
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
	return "ghcr.io/cosmos/gaia"
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
	if networkMode == "mainnet" {
		args = append(args, "--chain-id", mainnetChainID)
	} else if networkMode == "testnet" {
		args = append(args, "--chain-id", testnetChainID)
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
	return nil, nil
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
	switch networkType {
	case "mainnet":
		return mainnetSnapshot
	case "testnet":
		return testnetSnapshot
	default:
		return ""
	}
}

func (n *CosmosNetwork) RPCEndpoint(networkType string) string {
	switch networkType {
	case "mainnet":
		return mainnetRPC
	case "testnet":
		return testnetRPC
	default:
		return ""
	}
}

func (n *CosmosNetwork) AvailableNetworks() []string {
	return []string{"mainnet", "testnet"}
}
