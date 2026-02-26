package cosmos

import (
	"strings"

	rpcinternal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/rpc"
)

func (n *CosmosNetwork) resolveRPCEndpoint(raw, networkType string) string {
	fallback := ""
	if strings.TrimSpace(raw) == "" {
		profile, err := n.requireNetworkProfile(networkType)
		if err != nil {
			return ""
		}
		fallback = profile.RPCEndpoint
	}

	return rpcinternal.ResolveRPCEndpoint(raw, fallback, n.restToRPCHostMap)
}

func (n *CosmosNetwork) resolveRESTEndpoint(raw, networkType string) string {
	fallback := ""
	if strings.TrimSpace(raw) == "" {
		profile, err := n.requireNetworkProfile(networkType)
		if err != nil {
			return ""
		}
		fallback = profile.RESTEndpoint
	}

	return rpcinternal.ResolveRESTEndpoint(raw, fallback, n.rpcToRESTHostMap)
}
