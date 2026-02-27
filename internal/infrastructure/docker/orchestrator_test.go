package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/domain/ports"
	"github.com/altuslabsxyz/devnet-builder/internal/output"
)

type mockNetworkManager struct {
	createNetworkFunc   func(ctx context.Context, devnetName string) (string, string, error)
	deleteNetworkFunc   func(ctx context.Context, networkID string) error
	networkExistsFunc   func(ctx context.Context, networkID string) (bool, error)
	getNetworkSubnetFun func(ctx context.Context, networkID string) (string, error)
	listNetworksFunc    func(ctx context.Context) ([]ports.NetworkInfo, error)
}

func (m *mockNetworkManager) CreateNetwork(ctx context.Context, devnetName string) (string, string, error) {
	if m.createNetworkFunc != nil {
		return m.createNetworkFunc(ctx, devnetName)
	}
	return "", "", nil
}

func (m *mockNetworkManager) DeleteNetwork(ctx context.Context, networkID string) error {
	if m.deleteNetworkFunc != nil {
		return m.deleteNetworkFunc(ctx, networkID)
	}
	return nil
}

func (m *mockNetworkManager) NetworkExists(ctx context.Context, networkID string) (bool, error) {
	if m.networkExistsFunc != nil {
		return m.networkExistsFunc(ctx, networkID)
	}
	return false, nil
}

func (m *mockNetworkManager) GetNetworkSubnet(ctx context.Context, networkID string) (string, error) {
	if m.getNetworkSubnetFun != nil {
		return m.getNetworkSubnetFun(ctx, networkID)
	}
	return "", nil
}

func (m *mockNetworkManager) ListDevnetNetworks(ctx context.Context) ([]ports.NetworkInfo, error) {
	if m.listNetworksFunc != nil {
		return m.listNetworksFunc(ctx)
	}
	return nil, nil
}

type mockPortAllocator struct {
	allocateRangeFunc           func(ctx context.Context, devnetName string, validatorCount int) (*ports.PortAllocation, error)
	releaseRangeFunc            func(ctx context.Context, devnetName string) error
	getAllocationFunc           func(ctx context.Context, devnetName string) (*ports.PortAllocation, error)
	validatePortAvailabilityFun func(ctx context.Context, allocation *ports.PortAllocation) ([]int, error)
	listAllocationsFunc         func(ctx context.Context) ([]*ports.PortAllocation, error)
}

func (m *mockPortAllocator) AllocateRange(ctx context.Context, devnetName string, validatorCount int) (*ports.PortAllocation, error) {
	if m.allocateRangeFunc != nil {
		return m.allocateRangeFunc(ctx, devnetName, validatorCount)
	}
	return nil, nil
}

func (m *mockPortAllocator) ReleaseRange(ctx context.Context, devnetName string) error {
	if m.releaseRangeFunc != nil {
		return m.releaseRangeFunc(ctx, devnetName)
	}
	return nil
}

func (m *mockPortAllocator) GetAllocation(ctx context.Context, devnetName string) (*ports.PortAllocation, error) {
	if m.getAllocationFunc != nil {
		return m.getAllocationFunc(ctx, devnetName)
	}
	return nil, nil
}

func (m *mockPortAllocator) ValidatePortAvailability(ctx context.Context, allocation *ports.PortAllocation) ([]int, error) {
	if m.validatePortAvailabilityFun != nil {
		return m.validatePortAvailabilityFun(ctx, allocation)
	}
	return nil, nil
}

func (m *mockPortAllocator) ListAllocations(ctx context.Context) ([]*ports.PortAllocation, error) {
	if m.listAllocationsFunc != nil {
		return m.listAllocationsFunc(ctx)
	}
	return nil, nil
}

func newTestLogger() *output.Logger {
	l := output.NewLogger()
	l.SetJSONMode(true)
	return l
}

func TestOrchestratorRollback_AggregatesErrorsWithErrorsIs(t *testing.T) {
	errDeleteNetwork := errors.New("delete network failed")
	errReleaseRange := errors.New("release range failed")

	networkMgr := &mockNetworkManager{
		deleteNetworkFunc: func(ctx context.Context, networkID string) error { return errDeleteNetwork },
	}
	portAllocator := &mockPortAllocator{
		releaseRangeFunc: func(ctx context.Context, devnetName string) error { return errReleaseRange },
	}

	tmpDir := t.TempDir()
	o := NewOrchestratorWithStateDir(networkMgr, portAllocator, nil, tmpDir, newTestLogger())

	networkID := "network-1"
	state := &ports.DeploymentState{
		DevnetName: "devnet-a",
		NetworkID:  &networkID,
		PortRange: &ports.PortAllocation{
			DevnetName: "devnet-a",
		},
	}

	err := o.Rollback(context.Background(), state)
	if err == nil {
		t.Fatalf("expected aggregated rollback error")
	}
	if !errors.Is(err, errDeleteNetwork) {
		t.Fatalf("expected errors.Is(err, errDeleteNetwork) == true")
	}
	if !errors.Is(err, errReleaseRange) {
		t.Fatalf("expected errors.Is(err, errReleaseRange) == true")
	}
}

func TestOrchestratorRollback_SuccessDeletesStateFile(t *testing.T) {
	tmpDir := t.TempDir()
	o := NewOrchestratorWithStateDir(&mockNetworkManager{}, &mockPortAllocator{}, nil, tmpDir, newTestLogger())

	networkID := "network-ok"
	state := &ports.DeploymentState{
		DevnetName:        "devnet-ok",
		NetworkID:         &networkID,
		StartedContainers: []string{},
		HealthyContainers: []string{},
		Errors:            []ports.DeploymentError{},
		StartedAt:         time.Now(),
	}

	if err := o.saveState(state); err != nil {
		t.Fatalf("saveState failed: %v", err)
	}

	if err := o.Rollback(context.Background(), state); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	got, err := o.GetState(context.Background(), "devnet-ok")
	if err != nil {
		t.Fatalf("GetState unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected state file to be deleted after rollback")
	}
}

func TestOrchestratorHandleFailure_WrapsDeploymentAndRollbackErrors(t *testing.T) {
	errDeleteNetwork := errors.New("delete network failed")
	deploymentErr := errors.New("deployment failed")

	networkMgr := &mockNetworkManager{
		deleteNetworkFunc: func(ctx context.Context, networkID string) error { return errDeleteNetwork },
	}
	o := NewOrchestratorWithStateDir(networkMgr, &mockPortAllocator{}, nil, t.TempDir(), newTestLogger())

	networkID := "network-x"
	state := &ports.DeploymentState{DevnetName: "devnet-x", NetworkID: &networkID}

	err := o.handleFailure(context.Background(), state, deploymentErr)
	if err == nil {
		t.Fatalf("expected combined failure")
	}
	if !errors.Is(err, deploymentErr) {
		t.Fatalf("expected deployment error in chain")
	}
	if !errors.Is(err, errDeleteNetwork) {
		t.Fatalf("expected rollback error in chain")
	}
}
