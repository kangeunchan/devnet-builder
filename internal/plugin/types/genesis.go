// internal/plugin/types/genesis.go
package types

import (
	"time"

	appports "github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

// GenesisMode specifies how to obtain genesis
type GenesisMode = appports.GenesisMode

const (
	// GenesisModeRPC fetches genesis directly from RPC endpoint
	GenesisModeRPC GenesisMode = appports.GenesisModeRPC
	// GenesisModeSnapshot downloads snapshot and exports genesis from state
	GenesisModeSnapshot GenesisMode = appports.GenesisModeSnapshot
	// GenesisModeLocal uses a local genesis file
	GenesisModeLocal GenesisMode = appports.GenesisModeLocal
	// GenesisModeFresh generates a fresh genesis (no forking)
	GenesisModeFresh GenesisMode = appports.GenesisModeFresh
)

// GenesisSource specifies where to get genesis from
type GenesisSource = appports.GenesisSource

// ValidatorInfo represents validator information for genesis injection.
type ValidatorInfo = appports.ValidatorInfo

// GenesisPatchOptions specifies modifications to apply to genesis
type GenesisPatchOptions = appports.GenesisPatchOptions

// DefaultDevnetPatchOptions returns patch options suitable for local devnets
func DefaultDevnetPatchOptions(chainID string) GenesisPatchOptions {
	return GenesisPatchOptions{
		ChainID:       chainID,
		VotingPeriod:  30 * time.Second,
		UnbondingTime: 60 * time.Second,
		InflationRate: "0.0",
		MinGasPrice:   "0",
	}
}

// PluginGenesis handles network-specific genesis operations
type PluginGenesis interface {
	// GetRPCEndpoint returns the default RPC endpoint for a network type
	GetRPCEndpoint(networkType string) string

	// GetSnapshotURL returns the snapshot URL for a network type
	GetSnapshotURL(networkType string) string

	// ValidateGenesis validates genesis for this network
	ValidateGenesis(genesis []byte) error

	// PatchGenesis applies network-specific modifications to genesis
	// This is called after the generic patches from GenesisPatchOptions
	PatchGenesis(genesis []byte, opts GenesisPatchOptions) ([]byte, error)

	// ExportCommandArgs returns the command args for exporting genesis from snapshot
	// The binary path will be prepended by the caller
	ExportCommandArgs(homeDir string) []string

	// BinaryName returns the binary name for this network
	BinaryName() string
}

// FileBasedPluginGenesis extends PluginGenesis with file-based operations
// for handling large genesis files that exceed gRPC message size limits.
type FileBasedPluginGenesis interface {
	PluginGenesis

	// PatchGenesisFile applies network-specific modifications to a genesis file.
	// This is used for large genesis files that exceed gRPC message size limits.
	// inputPath: path to the input genesis file
	// outputPath: path where the modified genesis should be written
	// opts: patch options to apply
	// Returns the size of the output file in bytes, or an error.
	PatchGenesisFile(inputPath, outputPath string, opts GenesisPatchOptions) (int64, error)
}
