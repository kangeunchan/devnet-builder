package unit

import (
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
)

func TestRPCEndpoint_Fallbacks(t *testing.T) {
	networkModule := cosmos.New()

	if got := networkModule.RPCEndpoint("mainnet"); got != "https://cosmos-rpc.polkachu.com" {
		t.Fatalf("expected mainnet rpc endpoint, got %q", got)
	}
	if got := networkModule.RPCEndpoint("testnet"); got != "https://cosmos-testnet-rpc.polkachu.com" {
		t.Fatalf("expected testnet rpc endpoint, got %q", got)
	}
}

func TestSnapshotURL_Fallbacks(t *testing.T) {
	networkModule := cosmos.New()

	if got := networkModule.SnapshotURL("mainnet"); got != "https://snapshots.cosmos.directory/cosmoshub-4/latest.tar.lz4" {
		t.Fatalf("expected mainnet snapshot URL, got %q", got)
	}
	if got := networkModule.SnapshotURL("testnet"); got != "https://snapshots.kjnodes.com/cosmoshub-testnet/snapshot_latest.tar.lz4" {
		t.Fatalf("expected testnet snapshot URL, got %q", got)
	}
}
