package cosmos

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	networkMainnet = "mainnet"
	networkTestnet = "testnet"
)

// networkProfile holds all network-specific defaults in one place.
// This is the primary customization point for users copying this example.
type networkProfile struct {
	ChainID          string
	RPCEndpoint      string
	RESTEndpoint     string
	SnapshotIndexURL string
}

var networkProfiles = map[string]networkProfile{
	networkMainnet: {
		ChainID:          "cosmoshub-4",
		RPCEndpoint:      "https://cosmoshub.rpc.kjnodes.com",
		RESTEndpoint:     "https://cosmoshub.api.kjnodes.com",
		SnapshotIndexURL: "https://www.polkachu.com/tendermint_snapshots/cosmos",
	},
	networkTestnet: {
		ChainID:          "provider",
		RPCEndpoint:      "https://cosmoshub-testnet.rpc.kjnodes.com",
		RESTEndpoint:     "https://cosmoshub-testnet.api.kjnodes.com",
		SnapshotIndexURL: "https://www.polkachu.com/testnets/cosmos/snapshots",
	},
}

func canonicalNetworkType(networkType string) string {
	switch strings.ToLower(strings.TrimSpace(networkType)) {
	case networkMainnet:
		return networkMainnet
	case networkTestnet:
		return networkTestnet
	default:
		return ""
	}
}

func networkProfileByType(networkType string) (networkProfile, bool) {
	normalized := canonicalNetworkType(networkType)
	if normalized == "" {
		return networkProfile{}, false
	}

	profile, ok := networkProfiles[normalized]
	return profile, ok
}

func requireNetworkProfile(networkType string) (networkProfile, error) {
	if profile, ok := networkProfileByType(networkType); ok {
		return profile, nil
	}
	return networkProfile{}, fmt.Errorf("unsupported network type %q", strings.TrimSpace(networkType))
}

func allNetworkProfiles() []networkProfile {
	out := make([]networkProfile, 0, len(networkProfiles))
	for _, profile := range networkProfiles {
		out = append(out, profile)
	}
	return out
}

func endpointHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Hostname()))
}
