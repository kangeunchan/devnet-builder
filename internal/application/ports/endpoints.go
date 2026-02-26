package ports

import (
	"fmt"
	"strings"
)

const loopbackHost = "127.0.0.1"

// LoopbackRPCURL returns an HTTP URL for the local loopback host and port.
func LoopbackRPCURL(port int) string {
	return fmt.Sprintf("http://%s:%d", loopbackHost, port)
}

// NodeRPCEndpoint returns the canonical RPC endpoint URL for a node metadata.
func NodeRPCEndpoint(node *NodeMetadata) string {
	if node == nil {
		return ""
	}
	return LoopbackRPCURL(node.Ports.RPC)
}

// NodeEVMRPCEndpoint returns the canonical EVM RPC endpoint URL for a node metadata.
func NodeEVMRPCEndpoint(node *NodeMetadata) string {
	if node == nil {
		return ""
	}
	return LoopbackRPCURL(node.Ports.EVMRPC)
}

// NormalizeContainerNetworkName normalizes blockchain network name used for
// docker container naming.
func NormalizeContainerNetworkName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return "stable"
	}
	return normalized
}
