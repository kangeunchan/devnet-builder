package unit

import (
	"context"
	"errors"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func TestSnapshotURL_UsesHookResolver(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithHooks(cosmos.Hooks{
		SnapshotURLResolver: func(ctx context.Context, networkType string, profile cosmos.NetworkProfileConfig, cfg cosmos.Customization) (string, error) {
			_ = ctx
			_ = profile
			_ = cfg
			switch networkType {
			case "mainnet":
				return "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_30000001.tar.lz4", nil
			case "testnet":
				return "https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4", nil
			default:
				return "", nil
			}
		},
	}))

	if got := networkModule.SnapshotURL("mainnet"); got != "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_30000001.tar.lz4" {
		t.Fatalf("SnapshotURL(mainnet) = %q", got)
	}
	if got := networkModule.SnapshotURL("testnet"); got != "https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4" {
		t.Fatalf("SnapshotURL(testnet) = %q", got)
	}
}

func TestSnapshotURL_ReturnsEmptyOnResolverError(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithHooks(cosmos.Hooks{
		SnapshotURLResolver: func(ctx context.Context, networkType string, profile cosmos.NetworkProfileConfig, cfg cosmos.Customization) (string, error) {
			_ = ctx
			_ = networkType
			_ = profile
			_ = cfg
			return "", errors.New("resolver failed")
		},
	}))

	if got := networkModule.SnapshotURL("mainnet"); got != "" {
		t.Fatalf("SnapshotURL(mainnet) = %q, want empty", got)
	}
	if got := networkModule.SnapshotURL("unknown"); got != "" {
		t.Fatalf("SnapshotURL(unknown) = %q, want empty", got)
	}
}
