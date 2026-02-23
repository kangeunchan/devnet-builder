package cosmos

import (
	"context"

	devnetinternal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/devnet"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// GenerateDevnet builds a local validator set and writes the resulting genesis
// to <output_dir>/genesis.json. genesisFile is treated as an optional template
// input path only.
func (n *CosmosNetwork) GenerateDevnet(ctx context.Context, config network.GeneratorConfig, genesisFile string) error {
	ctx = ensureContext(ctx)

	return devnetinternal.Generate(ctx, devnetinternal.GenerateConfig{
		Runner:              devnetinternal.Runner(n.commandRunner()),
		Binary:              n.BinaryName(),
		Config:              config,
		Defaults:            n.DefaultGeneratorConfig(),
		TemplateGenesisPath: genesisFile,
	})
}
