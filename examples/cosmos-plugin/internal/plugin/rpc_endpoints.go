package cosmos

import (
	"net"
	"net/url"
	"strings"
)

func (n *CosmosNetwork) resolveRPCEndpoint(raw, networkType string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		if networkType == "testnet" {
			return testnetRPC
		}
		return mainnetRPC
	}

	endpoint = normalizeEndpoint(endpoint)

	if strings.Contains(endpoint, "cosmos-api.polkachu.com") {
		return strings.Replace(endpoint, "cosmos-api.polkachu.com", "cosmos-rpc.polkachu.com", 1)
	}
	if strings.Contains(endpoint, "cosmos-testnet-api.polkachu.com") {
		return strings.Replace(endpoint, "cosmos-testnet-api.polkachu.com", "cosmos-testnet-rpc.polkachu.com", 1)
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err == nil {
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
		if networkType == "testnet" {
			return testnetREST
		}
		return mainnetREST
	}

	endpoint = normalizeEndpoint(endpoint)

	if strings.Contains(endpoint, "cosmos-rpc.polkachu.com") {
		return strings.Replace(endpoint, "cosmos-rpc.polkachu.com", "cosmos-api.polkachu.com", 1)
	}
	if strings.Contains(endpoint, "cosmos-testnet-rpc.polkachu.com") {
		return strings.Replace(endpoint, "cosmos-testnet-rpc.polkachu.com", "cosmos-testnet-api.polkachu.com", 1)
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err == nil {
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
