package unit

import (
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
)

func TestStartCommand_UsesNetworkChainID(t *testing.T) {
	networkModule := cosmos.New()

	mainnet := networkModule.StartCommand("/tmp/node0", "mainnet")
	if !containsArgPair(mainnet, "--chain-id", "cosmoshub-4") {
		t.Fatalf("expected mainnet start command to include cosmoshub-4 chain-id, got %v", mainnet)
	}

	testnet := networkModule.StartCommand("/tmp/node0", "testnet")
	if !containsArgPair(testnet, "--chain-id", "provider") {
		t.Fatalf("expected testnet start command to include provider chain-id, got %v", testnet)
	}

	unknown := networkModule.StartCommand("/tmp/node0", "")
	if containsArg(unknown, "--chain-id") {
		t.Fatalf("expected default start command without chain-id override, got %v", unknown)
	}
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, key string, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
