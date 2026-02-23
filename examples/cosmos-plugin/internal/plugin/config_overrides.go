package cosmos

import (
	nodeconfig "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/nodeconfig"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// ============================================
// Node config overrides
// ============================================

func (n *CosmosNetwork) GetConfigOverrides(nodeIndex int, opts network.NodeConfigOptions) ([]byte, []byte, error) {
	builder := nodeconfig.Builder{BaseDenom: n.BaseDenom()}
	return builder.Build(nodeIndex, opts)
}
