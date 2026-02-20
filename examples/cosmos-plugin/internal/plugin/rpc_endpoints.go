package cosmos

import (
	"net"
	"net/url"
	"strings"
)

func (n *CosmosNetwork) resolveRPCEndpoint(raw, networkType string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return defaultNetworkProfile(networkType).RPCEndpoint
	}

	endpoint = normalizeEndpoint(endpoint)
	endpoint = replaceKnownHost(endpoint, restToRPCHostMap())

	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}

	if host, port, splitErr := net.SplitHostPort(u.Host); splitErr == nil {
		if port == "1317" {
			u.Host = net.JoinHostPort(host, "26657")
		}
		return strings.TrimRight(u.String(), "/")
	}

	if isLocalHost(u.Host) {
		u.Host = net.JoinHostPort(u.Host, "26657")
	}

	return strings.TrimRight(u.String(), "/")
}

func (n *CosmosNetwork) resolveRESTEndpoint(raw, networkType string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return defaultNetworkProfile(networkType).RESTEndpoint
	}

	endpoint = normalizeEndpoint(endpoint)
	endpoint = replaceKnownHost(endpoint, rpcToRESTHostMap())

	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}

	if host, port, splitErr := net.SplitHostPort(u.Host); splitErr == nil {
		if port == "26657" {
			u.Host = net.JoinHostPort(host, "1317")
		}
		return strings.TrimRight(u.String(), "/")
	}

	if isLocalHost(u.Host) {
		u.Host = net.JoinHostPort(u.Host, "1317")
	}

	return strings.TrimRight(u.String(), "/")
}

func normalizeEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return strings.TrimRight(raw, "/")
}

func isLocalHost(host string) bool {
	h := host
	if strings.Contains(h, ":") {
		if parsedHost, _, err := net.SplitHostPort(h); err == nil {
			h = parsedHost
		}
	}
	return h == "localhost" || h == "127.0.0.1"
}

func restToRPCHostMap() map[string]string {
	mapping := make(map[string]string, len(networkProfiles))
	for _, profile := range allNetworkProfiles() {
		restHost := endpointHost(profile.RESTEndpoint)
		rpcHost := endpointHost(profile.RPCEndpoint)
		if restHost == "" || rpcHost == "" {
			continue
		}
		mapping[restHost] = rpcHost
	}
	return mapping
}

func rpcToRESTHostMap() map[string]string {
	mapping := make(map[string]string, len(networkProfiles))
	for _, profile := range allNetworkProfiles() {
		restHost := endpointHost(profile.RESTEndpoint)
		rpcHost := endpointHost(profile.RPCEndpoint)
		if restHost == "" || rpcHost == "" {
			continue
		}
		mapping[rpcHost] = restHost
	}
	return mapping
}

func replaceKnownHost(raw string, hostMap map[string]string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	hostname := strings.ToLower(strings.TrimSpace(u.Hostname()))
	updatedHost, ok := hostMap[hostname]
	if !ok || updatedHost == "" {
		return raw
	}

	u.Host = updatedHost
	return strings.TrimRight(u.String(), "/")
}
