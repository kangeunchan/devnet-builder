package node

import "testing"

func TestNewDockerNodeManager_PropagatesNodeBinaryName(t *testing.T) {
	n := &Node{BinaryName: "gaiad"}

	manager := NewDockerNodeManager(n, "ghcr.io/cosmos/gaia:v25.3.2", "118", "/tmp/genesis.json", nil)
	if manager == nil {
		t.Fatalf("NewDockerNodeManager() returned nil")
	}

	if got := manager.manager.BinaryName; got != "gaiad" {
		t.Fatalf("Docker manager binary name = %q, want %q", got, "gaiad")
	}
	if got := manager.manager.HomeDir; got != DefaultDockerHomeDir {
		t.Fatalf("Docker manager home dir = %q, want %q", got, DefaultDockerHomeDir)
	}
}

func TestNewDockerNodeManagerWithConfig_UsesExplicitRuntimeSettings(t *testing.T) {
	n := &Node{BinaryName: "stabled"}

	manager := NewDockerNodeManagerWithConfig(DockerNodeManagerConfig{
		Node:        n,
		Image:       "ghcr.io/cosmos/gaia:v25.3.2",
		EVMChainID:  "118",
		GenesisPath: "/tmp/genesis.json",
		BinaryName:  "chaind",
		HomeDir:     "/home/gaia",
	})
	if manager == nil {
		t.Fatalf("NewDockerNodeManagerWithConfig() returned nil")
	}

	if got := manager.manager.BinaryName; got != "chaind" {
		t.Fatalf("Docker manager binary name = %q, want %q", got, "chaind")
	}
	if got := manager.manager.HomeDir; got != "/home/gaia" {
		t.Fatalf("Docker manager home dir = %q, want %q", got, "/home/gaia")
	}
}
