package network

import (
	"context"
	"strings"
	"testing"

	pkgNetwork "github.com/altuslabsxyz/devnet-builder/pkg/network"
	pb "github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type captureGenesisModule struct {
	pkgNetwork.Module
	lastOpts pkgNetwork.GenesisOptions
}

func TestPluginAdapter_ModifyGenesis_InvalidCoinReturnsError(t *testing.T) {
	module := &captureGenesisModule{}
	adapter := NewPluginAdapter(module)

	_, err := adapter.ModifyGenesis([]byte(`{"chain_id":"x"}`), GenesisOptions{
		ChainID: "cosmosdevnet-1",
		Accounts: []GenesisAccount{
			{
				Name:    "account0",
				Address: "cosmos1abc",
				Coins: []Coin{
					{Denom: "uatom", Amount: "bad-amount"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid coin amount")
	}
	if !strings.Contains(err.Error(), "invalid genesis account") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginAdapter_ModifyGenesis_BalanceAndCoinsMismatchReturnsError(t *testing.T) {
	module := &captureGenesisModule{}
	adapter := NewPluginAdapter(module)

	_, err := adapter.ModifyGenesis([]byte(`{"chain_id":"x"}`), GenesisOptions{
		ChainID: "cosmosdevnet-1",
		Accounts: []GenesisAccount{
			{
				Name:    "account0",
				Address: "cosmos1abc",
				Balance: "1000000uatom",
				Coins: []Coin{
					{Denom: "uatom", Amount: "2000000"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for balance/coins mismatch")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginAdapter_ModifyGenesis_InvalidBalancePassesThrough(t *testing.T) {
	module := &captureGenesisModule{}
	adapter := NewPluginAdapter(module)

	_, err := adapter.ModifyGenesis([]byte(`{"chain_id":"x"}`), GenesisOptions{
		ChainID: "cosmosdevnet-1",
		Accounts: []GenesisAccount{
			{
				Name:    "account0",
				Address: "cosmos1abc",
				Balance: "invalid-balance",
			},
		},
	})
	if err != nil {
		t.Fatalf("ModifyGenesis returned error: %v", err)
	}
	if len(module.lastOpts.AddAccounts) != 1 {
		t.Fatalf("expected 1 mapped account, got %d", len(module.lastOpts.AddAccounts))
	}
	if module.lastOpts.AddAccounts[0].Balance != "invalid-balance" {
		t.Fatalf("unexpected account balance: %q", module.lastOpts.AddAccounts[0].Balance)
	}
}

func TestPluginAdapter_ModifyGenesis_MissingAccountAddressReturnsError(t *testing.T) {
	module := &captureGenesisModule{}
	adapter := NewPluginAdapter(module)

	_, err := adapter.ModifyGenesis([]byte(`{"chain_id":"x"}`), GenesisOptions{
		ChainID: "cosmosdevnet-1",
		Accounts: []GenesisAccount{
			{
				Name:    "account0",
				Address: "   ",
				Balance: "1000000uatom",
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing account address")
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginAdapter_ModifyGenesis_MissingValidatorOperatorAddressReturnsError(t *testing.T) {
	module := &captureGenesisModule{}
	adapter := NewPluginAdapter(module)

	_, err := adapter.ModifyGenesis([]byte(`{"chain_id":"x"}`), GenesisOptions{
		ChainID: "cosmosdevnet-1",
		Validators: []GenesisValidatorInfo{
			{
				Moniker:         "validator0",
				OperatorAddress: " ",
				ConsPubKey:      "pubkey",
				SelfDelegation:  "1000000",
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing validator operator address")
	}
	if !strings.Contains(err.Error(), "operator_address is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func (m *captureGenesisModule) ModifyGenesis(genesis []byte, opts pkgNetwork.GenesisOptions) ([]byte, error) {
	m.lastOpts = opts
	return genesis, nil
}

type rpcAwareCaptureModule struct {
	*captureGenesisModule
}

func (m *rpcAwareCaptureModule) GetBlockHeight(ctx context.Context, rpcEndpoint string) (*pb.BlockHeightResponse, error) {
	return &pb.BlockHeightResponse{Height: 77}, nil
}

func TestPluginAdapter_ModifyGenesis_MapsAddAccounts(t *testing.T) {
	module := &captureGenesisModule{}
	adapter := NewPluginAdapter(module)

	_, err := adapter.ModifyGenesis([]byte(`{"chain_id":"x"}`), GenesisOptions{
		ChainID:       "cosmosdevnet-1",
		NumValidators: 1,
		Validators: []GenesisValidatorInfo{
			{
				Moniker:         "validator0",
				ConsPubKey:      "pubkey",
				OperatorAddress: "cosmosvaloper1abc",
				SelfDelegation:  "1000000",
			},
		},
		Accounts: []GenesisAccount{
			{
				Name:    "account0",
				Address: "cosmos1abc",
				Balance: "2500000uatom",
			},
		},
	})
	if err != nil {
		t.Fatalf("ModifyGenesis returned error: %v", err)
	}

	if len(module.lastOpts.AddAccounts) != 1 {
		t.Fatalf("expected 1 mapped account, got %d", len(module.lastOpts.AddAccounts))
	}
	account := module.lastOpts.AddAccounts[0]
	if account.Name != "account0" {
		t.Fatalf("unexpected account name: %q", account.Name)
	}
	if account.Address != "cosmos1abc" {
		t.Fatalf("unexpected account address: %q", account.Address)
	}
	if account.Balance != "2500000uatom" {
		t.Fatalf("unexpected account balance: %q", account.Balance)
	}
}

func TestPluginAdapter_GetBlockHeight_Unimplemented(t *testing.T) {
	adapter := NewPluginAdapter(&captureGenesisModule{})

	_, err := adapter.GetBlockHeight(context.Background(), "http://localhost:26657")
	if err == nil {
		t.Fatal("expected unimplemented error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Fatalf("unexpected gRPC code: got %v, want %v", st.Code(), codes.Unimplemented)
	}
}

func TestPluginAdapter_GetBlockHeight_DelegatesToModule(t *testing.T) {
	module := &rpcAwareCaptureModule{captureGenesisModule: &captureGenesisModule{}}
	adapter := NewPluginAdapter(module)

	resp, err := adapter.GetBlockHeight(context.Background(), "http://localhost:26657")
	if err != nil {
		t.Fatalf("GetBlockHeight returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Height != 77 {
		t.Fatalf("unexpected height: %d", resp.Height)
	}
}

type endpointListModule struct {
	pkgNetwork.Module
}

func (m *endpointListModule) RPCEndpoint(networkType string) string {
	return "https://primary-rpc"
}

func (m *endpointListModule) RPCEndpoints(networkType string) []string {
	return []string{" https://primary-rpc ", "https://fallback-rpc", "https://primary-rpc", ""}
}

func (m *endpointListModule) SnapshotURL(networkType string) string {
	return "https://primary-snapshot"
}

func (m *endpointListModule) SnapshotURLs(networkType string) []string {
	return []string{"https://primary-snapshot", "https://fallback-snapshot", "https://primary-snapshot"}
}

func TestPluginAdapter_RPCEndpoints_UsesModuleListAndDeduplicates(t *testing.T) {
	adapter := NewPluginAdapter(&endpointListModule{})

	got := adapter.RPCEndpoints("mainnet")
	if len(got) != 2 {
		t.Fatalf("expected 2 RPC endpoints after dedupe, got %d (%v)", len(got), got)
	}
	if got[0] != "https://primary-rpc" || got[1] != "https://fallback-rpc" {
		t.Fatalf("unexpected RPC endpoints: %v", got)
	}
}

func TestPluginAdapter_SnapshotURLs_UsesModuleListAndDeduplicates(t *testing.T) {
	adapter := NewPluginAdapter(&endpointListModule{})

	got := adapter.SnapshotURLs("mainnet")
	if len(got) != 2 {
		t.Fatalf("expected 2 snapshot URLs after dedupe, got %d (%v)", len(got), got)
	}
	if got[0] != "https://primary-snapshot" || got[1] != "https://fallback-snapshot" {
		t.Fatalf("unexpected snapshot URLs: %v", got)
	}
}

type endpointPriorityModule struct {
	pkgNetwork.Module
}

func (m *endpointPriorityModule) RPCEndpoint(networkType string) string {
	return "https://legacy-rpc"
}

func (m *endpointPriorityModule) RPCEndpoints(networkType string) []string {
	return []string{" https://primary-rpc ", "https://fallback-rpc"}
}

func (m *endpointPriorityModule) SnapshotURL(networkType string) string {
	return "https://legacy-snapshot"
}

func (m *endpointPriorityModule) SnapshotURLs(networkType string) []string {
	return []string{"https://primary-snapshot", "https://fallback-snapshot"}
}

func TestPluginAdapter_RPCEndpoint_UsesFirstFromEndpointList(t *testing.T) {
	adapter := NewPluginAdapter(&endpointPriorityModule{})

	got := adapter.RPCEndpoint("mainnet")
	if got != "https://primary-rpc" {
		t.Fatalf("unexpected RPC endpoint: %q", got)
	}
}

func TestPluginAdapter_SnapshotURL_UsesFirstFromSnapshotList(t *testing.T) {
	adapter := NewPluginAdapter(&endpointPriorityModule{})

	got := adapter.SnapshotURL("mainnet")
	if got != "https://primary-snapshot" {
		t.Fatalf("unexpected snapshot URL: %q", got)
	}
}
