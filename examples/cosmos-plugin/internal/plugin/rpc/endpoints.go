package rpc

import (
	"net"
	"net/url"
	"strings"
)

func ResolveRPCEndpoint(raw, fallback string, restToRPCHostMap map[string]string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return strings.TrimSpace(fallback)
	}

	endpoint = normalizeEndpoint(endpoint)
	endpoint = replaceKnownHost(endpoint, restToRPCHostMap)

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
		u.Host = net.JoinHostPort(u.Hostname(), "26657")
	}

	return strings.TrimRight(u.String(), "/")
}

func ResolveRESTEndpoint(raw, fallback string, rpcToRESTHostMap map[string]string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return strings.TrimSpace(fallback)
	}

	endpoint = normalizeEndpoint(endpoint)
	endpoint = replaceKnownHost(endpoint, rpcToRESTHostMap)

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
		u.Host = net.JoinHostPort(u.Hostname(), "1317")
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
	h := strings.TrimSpace(host)
	if strings.Contains(h, ":") {
		if parsedHost, _, err := net.SplitHostPort(h); err == nil {
			h = parsedHost
		}
	}

	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}

	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
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
