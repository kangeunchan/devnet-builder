package application

import (
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

func TestDeriveOverallStatusFromHealth(t *testing.T) {
	tests := []struct {
		name     string
		health   *dto.HealthOutput
		metadata *ports.DevnetMetadata
		want     string
	}{
		{
			name: "all running",
			health: &dto.HealthOutput{
				Nodes: []dto.NodeHealthStatus{
					{Status: ports.NodeStatusRunning},
					{Status: ports.NodeStatusRunning},
				},
			},
			want: "running",
		},
		{
			name: "all syncing",
			health: &dto.HealthOutput{
				Nodes: []dto.NodeHealthStatus{
					{Status: ports.NodeStatusSyncing},
					{Status: ports.NodeStatusSyncing},
				},
			},
			want: "syncing",
		},
		{
			name: "running and syncing",
			health: &dto.HealthOutput{
				Nodes: []dto.NodeHealthStatus{
					{Status: ports.NodeStatusRunning},
					{Status: ports.NodeStatusSyncing},
				},
			},
			want: "syncing",
		},
		{
			name: "all errors",
			health: &dto.HealthOutput{
				Nodes: []dto.NodeHealthStatus{
					{Status: ports.NodeStatusError},
				},
			},
			want: "error",
		},
		{
			name: "all stopped",
			health: &dto.HealthOutput{
				Nodes: []dto.NodeHealthStatus{
					{Status: ports.NodeStatusStopped},
				},
			},
			want: "stopped",
		},
		{
			name:     "fallback to metadata",
			health:   nil,
			metadata: &ports.DevnetMetadata{Status: ports.StateProvisioned},
			want:     string(ports.StateProvisioned),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := deriveOverallStatusFromHealth(tc.health, tc.metadata)
			if got != tc.want {
				t.Fatalf("deriveOverallStatusFromHealth() = %q, want %q", got, tc.want)
			}
		})
	}
}
