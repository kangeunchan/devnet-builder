package di

import (
	"context"
	"errors"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	infranetwork "github.com/altuslabsxyz/devnet-builder/internal/infrastructure/network"
	infrarpc "github.com/altuslabsxyz/devnet-builder/internal/infrastructure/rpc"
	pb "github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type rpcAwareModule struct {
	infranetwork.NetworkModule
	called bool
}

func (m *rpcAwareModule) GetGovernanceParams(rpcEndpoint, networkType string) (*pb.GovernanceParamsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *rpcAwareModule) GetBlockHeight(ctx context.Context, rpcEndpoint string) (*pb.BlockHeightResponse, error) {
	m.called = true
	return &pb.BlockHeightResponse{Height: 321}, nil
}

func (m *rpcAwareModule) GetBlockTime(ctx context.Context, rpcEndpoint string, sampleSize int) (*pb.BlockTimeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *rpcAwareModule) IsChainRunning(ctx context.Context, rpcEndpoint string) (*pb.ChainStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *rpcAwareModule) WaitForBlock(ctx context.Context, rpcEndpoint string, targetHeight int64, timeoutMs int64) (*pb.WaitForBlockResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *rpcAwareModule) GetProposal(ctx context.Context, rpcEndpoint string, proposalID uint64) (*pb.ProposalResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *rpcAwareModule) GetUpgradePlan(ctx context.Context, rpcEndpoint string) (*pb.UpgradePlanResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *rpcAwareModule) GetAppVersion(ctx context.Context, rpcEndpoint string) (*pb.AppVersionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

var _ infrarpc.NetworkPluginModule = (*rpcAwareModule)(nil)

func TestToNetworkGenesisOptions_MapsAddAccounts(t *testing.T) {
	opts := ports.GenesisModifyOptions{
		ChainID:       "cosmosdevnet-1",
		NumValidators: 1,
		AddAccounts: []ports.AccountInfo{
			{
				Name:    "account0",
				Address: "cosmos1abc",
				Balance: "2500000uatom",
			},
		},
	}

	converted, err := toNetworkGenesisOptions(opts)
	if err != nil {
		t.Fatalf("toNetworkGenesisOptions returned error: %v", err)
	}
	if len(converted.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(converted.Accounts))
	}
	account := converted.Accounts[0]
	if account.Name != "account0" {
		t.Fatalf("unexpected account name: %q", account.Name)
	}
	if account.Address != "cosmos1abc" {
		t.Fatalf("unexpected account address: %q", account.Address)
	}
	if account.Balance != "2500000uatom" {
		t.Fatalf("unexpected account balance: %q", account.Balance)
	}
	if len(account.Coins) != 0 {
		t.Fatalf("expected no pre-parsed coins, got %d", len(account.Coins))
	}
}

func TestToNetworkGenesisOptions_PreservesAddAccountBalanceForPluginPolicy(t *testing.T) {
	converted, err := toNetworkGenesisOptions(ports.GenesisModifyOptions{
		AddAccounts: []ports.AccountInfo{
			{
				Name:    "account0",
				Address: "cosmos1abc",
				Balance: "invalid-balance",
			},
		},
	})
	if err != nil {
		t.Fatalf("toNetworkGenesisOptions returned error: %v", err)
	}
	if len(converted.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(converted.Accounts))
	}
	if converted.Accounts[0].Balance != "invalid-balance" {
		t.Fatalf("unexpected account balance: %q", converted.Accounts[0].Balance)
	}
}

func TestCreateRPCClient_WiresPluginModule(t *testing.T) {
	module := &rpcAwareModule{}
	factory := NewInfrastructureFactory(t.TempDir(), nil).WithNetworkModule(module)

	client := factory.CreateRPCClient("localhost", 26657)
	height, err := client.GetBlockHeight(context.Background())
	if err != nil {
		t.Fatalf("GetBlockHeight returned error: %v", err)
	}
	if height != 321 {
		t.Fatalf("unexpected height: %d", height)
	}
	if !module.called {
		t.Fatalf("expected plugin module GetBlockHeight to be called")
	}
}

type endpointAwareModule struct {
	infranetwork.NetworkModule
}

func (m *endpointAwareModule) RPCEndpoint(networkType string) string {
	return "https://primary-rpc"
}

func (m *endpointAwareModule) RPCEndpoints(networkType string) []string {
	return []string{"https://primary-rpc", "https://fallback-rpc", "https://primary-rpc"}
}

func (m *endpointAwareModule) SnapshotURL(networkType string) string {
	return "https://primary-snapshot"
}

func (m *endpointAwareModule) SnapshotURLs(networkType string) []string {
	return []string{"https://primary-snapshot", "https://fallback-snapshot", "https://primary-snapshot"}
}

type endpointPriorityModule struct {
	infranetwork.NetworkModule
}

func (m *endpointPriorityModule) RPCEndpoint(networkType string) string {
	return "https://legacy-rpc"
}

func (m *endpointPriorityModule) RPCEndpoints(networkType string) []string {
	return []string{"https://primary-rpc", "https://fallback-rpc"}
}

func (m *endpointPriorityModule) SnapshotURL(networkType string) string {
	return "https://legacy-snapshot"
}

func (m *endpointPriorityModule) SnapshotURLs(networkType string) []string {
	return []string{"https://primary-snapshot", "https://fallback-snapshot"}
}

func TestNetworkModuleAdapter_RPCEndpoints_UsesModuleList(t *testing.T) {
	adapter := &networkModuleAdapter{module: &endpointAwareModule{}}

	got := adapter.RPCEndpoints("mainnet")
	if len(got) != 2 {
		t.Fatalf("expected 2 deduplicated RPC endpoints, got %d (%v)", len(got), got)
	}
	if got[0] != "https://primary-rpc" || got[1] != "https://fallback-rpc" {
		t.Fatalf("unexpected RPC endpoints: %v", got)
	}
}

func TestNetworkModuleAdapter_SnapshotURLs_UsesModuleList(t *testing.T) {
	adapter := &networkModuleAdapter{module: &endpointAwareModule{}}

	got := adapter.SnapshotURLs("mainnet")
	if len(got) != 2 {
		t.Fatalf("expected 2 deduplicated snapshot URLs, got %d (%v)", len(got), got)
	}
	if got[0] != "https://primary-snapshot" || got[1] != "https://fallback-snapshot" {
		t.Fatalf("unexpected snapshot URLs: %v", got)
	}
}

func TestNetworkModuleAdapter_RPCEndpoint_UsesFirstFromEndpointList(t *testing.T) {
	adapter := &networkModuleAdapter{module: &endpointPriorityModule{}}

	got := adapter.RPCEndpoint("mainnet")
	if got != "https://primary-rpc" {
		t.Fatalf("unexpected RPC endpoint: %q", got)
	}
}

func TestNetworkModuleAdapter_SnapshotURL_UsesFirstFromSnapshotList(t *testing.T) {
	adapter := &networkModuleAdapter{module: &endpointPriorityModule{}}

	got := adapter.SnapshotURL("mainnet")
	if got != "https://primary-snapshot" {
		t.Fatalf("unexpected snapshot URL: %q", got)
	}
}

func TestHealthCheckerAdapter_MapsDockerRunningNodeToSyncing(t *testing.T) {
	oldInspect := dockerInspectCommand
	dockerInspectCommand = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("true|running"), nil
	}
	t.Cleanup(func() {
		dockerInspectCommand = oldInspect
	})

	adapter := &healthCheckerAdapter{
		factory: &InfrastructureFactory{dockerMode: true},
	}
	node := &ports.NodeMetadata{
		Index:       0,
		Name:        "node0",
		ContainerID: "container-1",
	}
	status := &ports.HealthStatus{
		Status: ports.NodeStatusError,
		Error:  errors.New("rpc unavailable"),
	}

	adapter.applyDockerBootstrapFallback(context.Background(), node, status)

	if status.Status != ports.NodeStatusSyncing {
		t.Fatalf("expected syncing status, got %s", status.Status)
	}
	if !status.IsRunning {
		t.Fatalf("expected IsRunning=true")
	}
	if status.Error != nil {
		t.Fatalf("expected error cleared, got %v", status.Error)
	}
}

func TestHealthCheckerAdapter_DoesNotOverrideStoppedContainer(t *testing.T) {
	oldInspect := dockerInspectCommand
	dockerInspectCommand = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("false|exited"), nil
	}
	t.Cleanup(func() {
		dockerInspectCommand = oldInspect
	})

	adapter := &healthCheckerAdapter{
		factory: &InfrastructureFactory{dockerMode: true},
	}
	node := &ports.NodeMetadata{
		Index:       0,
		Name:        "node0",
		ContainerID: "container-1",
	}
	originalErr := errors.New("rpc unavailable")
	status := &ports.HealthStatus{
		Status: ports.NodeStatusError,
		Error:  originalErr,
	}

	adapter.applyDockerBootstrapFallback(context.Background(), node, status)

	if status.Status != ports.NodeStatusError {
		t.Fatalf("expected error status to remain, got %s", status.Status)
	}
	if status.Error != originalErr {
		t.Fatalf("expected original error to remain")
	}
}
