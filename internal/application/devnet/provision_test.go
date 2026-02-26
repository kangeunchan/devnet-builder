package devnet

import (
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

type endpointAwareNetworkModule struct {
	ports.NetworkModule
}

func (m *endpointAwareNetworkModule) RPCEndpoint(networkType string) string {
	return " https://primary-rpc "
}

func (m *endpointAwareNetworkModule) SnapshotURL(networkType string) string {
	return "https://primary-snapshot"
}

func TestProvisionUseCase_ResolveRPCEndpoint_TrimsValue(t *testing.T) {
	uc := &ProvisionUseCase{
		networkModule: &endpointAwareNetworkModule{},
	}

	got := uc.resolveRPCEndpoint("mainnet")
	if got != "https://primary-rpc" {
		t.Fatalf("unexpected endpoint: %q", got)
	}
}

func TestProvisionUseCase_ResolveSnapshotURL_OverrideFirst(t *testing.T) {
	uc := &ProvisionUseCase{
		networkModule: &endpointAwareNetworkModule{},
	}

	got := uc.resolveSnapshotURL(dto.ProvisionInput{
		Network:     "mainnet",
		SnapshotURL: "https://override-snapshot",
	})
	if got != "https://override-snapshot" {
		t.Fatalf("expected override snapshot URL, got %q", got)
	}
}
