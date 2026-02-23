package cosmos

import (
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

func buildNetworkProfiles(custom map[string]NetworkProfileConfig) map[string]networkProfile {
	out := make(map[string]networkProfile, len(custom))
	for key, cfg := range custom {
		normalized := canonicalNetworkType(key)
		if normalized == "" {
			normalized = strings.ToLower(strings.TrimSpace(key))
		}
		if normalized == "" {
			continue
		}

		if strings.TrimSpace(cfg.ChainID) == "" || strings.TrimSpace(cfg.RPCEndpoint) == "" || strings.TrimSpace(cfg.RESTEndpoint) == "" {
			continue
		}

		out[normalized] = networkProfile{
			ChainID:          strings.TrimSpace(cfg.ChainID),
			RPCEndpoint:      strings.TrimSpace(cfg.RPCEndpoint),
			RESTEndpoint:     strings.TrimSpace(cfg.RESTEndpoint),
			SnapshotIndexURL: strings.TrimSpace(cfg.SnapshotIndexURL),
		}
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

func buildRESTToRPCHostMapWithOverrides(profiles map[string]networkProfile, overrides map[string]string) map[string]string {
	mapping := make(map[string]string, len(profiles)+len(overrides))
	for _, profile := range profiles {
		restHost := endpointHost(profile.RESTEndpoint)
		rpcHost := endpointHost(profile.RPCEndpoint)
		if restHost == "" || rpcHost == "" {
			continue
		}
		mapping[restHost] = rpcHost
	}
	for key, value := range overrides {
		src := strings.ToLower(strings.TrimSpace(key))
		dst := strings.ToLower(strings.TrimSpace(value))
		if src == "" || dst == "" {
			continue
		}
		mapping[src] = dst
	}
	return mapping
}

func buildRPCToRESTHostMapWithOverrides(profiles map[string]networkProfile, overrides map[string]string) map[string]string {
	mapping := make(map[string]string, len(profiles)+len(overrides))
	for _, profile := range profiles {
		restHost := endpointHost(profile.RESTEndpoint)
		rpcHost := endpointHost(profile.RPCEndpoint)
		if restHost == "" || rpcHost == "" {
			continue
		}
		mapping[rpcHost] = restHost
	}
	for key, value := range overrides {
		src := strings.ToLower(strings.TrimSpace(key))
		dst := strings.ToLower(strings.TrimSpace(value))
		if src == "" || dst == "" {
			continue
		}
		mapping[src] = dst
	}
	return mapping
}
