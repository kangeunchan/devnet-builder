package unit

import (
	"context"
	"strings"
	"testing"
	"time"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func TestRPCEndpoint_UnsupportedNetworkReturnsEmpty(t *testing.T) {
	networkModule := cosmos.New()

	if got := networkModule.RPCEndpoint("unknown-network"); got != "" {
		t.Fatalf("RPCEndpoint(unknown-network) = %q, want empty", got)
	}
}

func TestGetBlockHeight_IPv6LoopbackEndpointUsesRPCPort(t *testing.T) {
	networkModule := cosmos.New()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resp, err := networkModule.GetBlockHeight(ctx, "http://[::1]")
	if err != nil {
		t.Fatalf("GetBlockHeight returned error: %v", err)
	}
	if resp == nil {
		t.Fatalf("GetBlockHeight returned nil response")
	}
	if resp.Error == "" {
		t.Fatalf("expected request failure error for loopback endpoint")
	}
	if !strings.Contains(resp.Error, "[::1]:26657/status") {
		t.Fatalf("expected RPC endpoint normalization to :26657, got: %q", resp.Error)
	}
}
