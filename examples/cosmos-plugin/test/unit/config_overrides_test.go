package unit

import (
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
	"github.com/pelletier/go-toml/v2"
)

func TestGetConfigOverrides_UsesDefaults(t *testing.T) {
	networkModule := cosmos.New()

	opts := network.NodeConfigOptions{
		Moniker:         "",
		PersistentPeers: "peer1@127.0.0.1:26656",
		Ports: network.PortConfig{
			RPC:  26657,
			P2P:  26656,
			API:  1317,
			GRPC: 9090,
		},
	}

	configToml, appToml, err := networkModule.GetConfigOverrides(2, opts)
	if err != nil {
		t.Fatalf("GetConfigOverrides returned error: %v", err)
	}

	var configMap map[string]any
	if err := toml.Unmarshal(configToml, &configMap); err != nil {
		t.Fatalf("failed to parse config.toml overrides: %v", err)
	}
	if got := configMap["moniker"]; got != "node2" {
		t.Fatalf("expected fallback moniker node2, got: %v", got)
	}
	if got := configMap["db_backend"]; got != "pebbledb" {
		t.Fatalf("expected db_backend=pebbledb, got: %v", got)
	}
	configP2P, _ := asMap(configMap["p2p"])
	if got := configP2P["persistent_peers"]; got != "peer1@127.0.0.1:26656" {
		t.Fatalf("expected persistent peers, got: %v", got)
	}
	configRPC, _ := asMap(configMap["rpc"])
	if got := configRPC["laddr"]; got != "tcp://0.0.0.0:26657" {
		t.Fatalf("expected rpc laddr override, got: %v", got)
	}
	configConsensus, _ := asMap(configMap["consensus"])
	if got := configConsensus["timeout_commit"]; got != "1s" {
		t.Fatalf("expected consensus timeout_commit override, got: %v", got)
	}

	var appMap map[string]any
	if err := toml.Unmarshal(appToml, &appMap); err != nil {
		t.Fatalf("failed to parse app.toml overrides: %v", err)
	}
	if got := appMap["minimum-gas-prices"]; got != "0uatom" {
		t.Fatalf("expected base denom-derived minimum-gas-prices, got: %v", got)
	}
	appAPI, _ := asMap(appMap["api"])
	if got := appAPI["address"]; got != "tcp://0.0.0.0:1317" {
		t.Fatalf("expected API address override, got: %v", got)
	}
	appGRPC, _ := asMap(appMap["grpc"])
	if got := appGRPC["address"]; got != "0.0.0.0:9090" {
		t.Fatalf("expected GRPC address override, got: %v", got)
	}
}
