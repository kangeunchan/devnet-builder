package unit

import (
	"context"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func TestRPCEndpoint_Defaults(t *testing.T) {
	networkModule := cosmos.New()

	if got := networkModule.RPCEndpoint("mainnet"); got != "https://cosmos-rpc.polkachu.com" {
		t.Fatalf("expected mainnet rpc endpoint, got %q", got)
	}
	if got := networkModule.RPCEndpoint("testnet"); got != "https://cosmos-testnet-rpc.polkachu.com" {
		t.Fatalf("expected testnet rpc endpoint, got %q", got)
	}
}

func TestSnapshotURL_Defaults(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithHooks(cosmos.Hooks{
		SnapshotURLResolver: func(ctx context.Context, networkType string, profile cosmos.NetworkProfileConfig, cfg cosmos.Customization) (string, error) {
			_ = ctx
			_ = profile
			_ = cfg
			switch networkType {
			case "mainnet":
				return "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_123.tar.lz4", nil
			case "testnet":
				return "https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_456.tar.lz4", nil
			default:
				return "", nil
			}
		},
	}))

	if got := networkModule.SnapshotURL("mainnet"); got != "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_123.tar.lz4" {
		t.Fatalf("expected mainnet snapshot URL, got %q", got)
	}
	if got := networkModule.SnapshotURL("testnet"); got != "https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_456.tar.lz4" {
		t.Fatalf("expected testnet snapshot URL, got %q", got)
	}
}

func TestRPCEndpoints_ContainOnlyPrimary(t *testing.T) {
	networkModule := cosmos.New()

	mainnetEndpoints := networkModule.RPCEndpoints("mainnet")
	if len(mainnetEndpoints) != 1 {
		t.Fatalf("expected 1 mainnet RPC endpoint, got %d", len(mainnetEndpoints))
	}
	if mainnetEndpoints[0] != networkModule.RPCEndpoint("mainnet") {
		t.Fatalf("primary mainnet RPC endpoint mismatch: list=%q primary=%q", mainnetEndpoints[0], networkModule.RPCEndpoint("mainnet"))
	}

	testnetEndpoints := networkModule.RPCEndpoints("testnet")
	if len(testnetEndpoints) != 1 {
		t.Fatalf("expected 1 testnet RPC endpoint, got %d", len(testnetEndpoints))
	}
	if testnetEndpoints[0] != networkModule.RPCEndpoint("testnet") {
		t.Fatalf("primary testnet RPC endpoint mismatch: list=%q primary=%q", testnetEndpoints[0], networkModule.RPCEndpoint("testnet"))
	}
}

func TestSnapshotURLs_ContainOnlyPrimary(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithHooks(cosmos.Hooks{
		SnapshotURLResolver: func(ctx context.Context, networkType string, profile cosmos.NetworkProfileConfig, cfg cosmos.Customization) (string, error) {
			_ = ctx
			_ = profile
			_ = cfg
			switch networkType {
			case "mainnet":
				return "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_123.tar.lz4", nil
			case "testnet":
				return "https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_456.tar.lz4", nil
			default:
				return "", nil
			}
		},
	}))

	mainnetURLs := networkModule.SnapshotURLs("mainnet")
	if len(mainnetURLs) != 1 {
		t.Fatalf("expected 1 mainnet snapshot URL, got %d", len(mainnetURLs))
	}
	if mainnetURLs[0] != networkModule.SnapshotURL("mainnet") {
		t.Fatalf("primary mainnet snapshot mismatch: list=%q primary=%q", mainnetURLs[0], networkModule.SnapshotURL("mainnet"))
	}

	testnetURLs := networkModule.SnapshotURLs("testnet")
	if len(testnetURLs) != 1 {
		t.Fatalf("expected 1 testnet snapshot URL, got %d", len(testnetURLs))
	}
	if testnetURLs[0] != networkModule.SnapshotURL("testnet") {
		t.Fatalf("primary testnet snapshot mismatch: list=%q primary=%q", testnetURLs[0], networkModule.SnapshotURL("testnet"))
	}
}
