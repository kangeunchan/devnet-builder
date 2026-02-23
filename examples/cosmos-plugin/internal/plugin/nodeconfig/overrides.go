package nodeconfig

import (
	"fmt"
	"strings"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
	"github.com/pelletier/go-toml/v2"
)

type Builder struct {
	BaseDenom string
}

func (b Builder) Build(nodeIndex int, opts network.NodeConfigOptions) ([]byte, []byte, error) {
	moniker := strings.TrimSpace(opts.Moniker)
	if moniker == "" {
		moniker = fmt.Sprintf("node%d", nodeIndex)
	}

	configToml, err := buildConfigTomlOverrides(moniker, opts)
	if err != nil {
		return nil, nil, err
	}
	appToml, err := buildAppTomlOverrides(b.BaseDenom, opts)
	if err != nil {
		return nil, nil, err
	}

	return configToml, appToml, nil
}

func buildConfigTomlOverrides(moniker string, opts network.NodeConfigOptions) ([]byte, error) {
	data := map[string]any{
		"moniker": moniker,
		"rpc": map[string]any{
			"laddr": tcpListenAddress("0.0.0.0", opts.Ports.RPC),
		},
		"p2p": map[string]any{
			"laddr":            tcpListenAddress("0.0.0.0", opts.Ports.P2P),
			"persistent_peers": strings.TrimSpace(opts.PersistentPeers),
		},
		"consensus": map[string]any{
			"timeout_commit": "1s",
		},
	}

	return marshalTOMLWithTrailingNewline(data)
}

func buildAppTomlOverrides(baseDenom string, opts network.NodeConfigOptions) ([]byte, error) {
	minimumGasPrices := fmt.Sprintf("0%s", baseDenom)

	data := map[string]any{
		"minimum-gas-prices": minimumGasPrices,
		"api": map[string]any{
			"enable":              true,
			"enabled-unsafe-cors": true,
			"address":             tcpListenAddress("0.0.0.0", opts.Ports.API),
		},
		"grpc": map[string]any{
			"enable":  true,
			"address": listenAddress("0.0.0.0", opts.Ports.GRPC),
		},
	}

	return marshalTOMLWithTrailingNewline(data)
}

func marshalTOMLWithTrailingNewline(v map[string]any) ([]byte, error) {
	out, err := toml.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}

func tcpListenAddress(host string, port int) string {
	return fmt.Sprintf("tcp://%s:%d", host, port)
}

func listenAddress(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}
