package unit

import (
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func TestStartCommand_UsesHomeOnly(t *testing.T) {
	networkModule := cosmos.New()

	mainnet := networkModule.StartCommand("/tmp/node0", "mainnet")
	if len(mainnet) != 5 || mainnet[0] != "start" || mainnet[1] != "--home" || mainnet[2] != "/tmp/node0" || mainnet[3] != "--iavl-disable-fastnode" || mainnet[4] != "--db_backend=pebbledb" {
		t.Fatalf("expected mainnet start command to be [start --home /tmp/node0 --iavl-disable-fastnode --db_backend=pebbledb], got %v", mainnet)
	}

	testnet := networkModule.StartCommand("/tmp/node0", "testnet")
	if len(testnet) != 5 || testnet[0] != "start" || testnet[1] != "--home" || testnet[2] != "/tmp/node0" || testnet[3] != "--iavl-disable-fastnode" || testnet[4] != "--db_backend=pebbledb" {
		t.Fatalf("expected testnet start command to be [start --home /tmp/node0 --iavl-disable-fastnode --db_backend=pebbledb], got %v", testnet)
	}

	unknown := networkModule.StartCommand("/tmp/node0", "")
	if len(unknown) != 5 || unknown[0] != "start" || unknown[1] != "--home" || unknown[2] != "/tmp/node0" || unknown[3] != "--iavl-disable-fastnode" || unknown[4] != "--db_backend=pebbledb" {
		t.Fatalf("expected default start command to be [start --home /tmp/node0 --iavl-disable-fastnode --db_backend=pebbledb], got %v", unknown)
	}
}

func containsArgPair(args []string, key string, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
