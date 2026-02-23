package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

type markerMutator struct{}

func (markerMutator) Name() string { return "marker-mutator" }

func (markerMutator) MutateModule(moduleName string, module map[string]any, opts network.GenesisOptions, cfg network.GenesisConfig) error {
	if moduleName != "gov" {
		return nil
	}
	module["marker"] = "hooked"
	return nil
}

func TestHooks_SnapshotURLResolverOverride(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithHooks(cosmos.Hooks{
		SnapshotURLResolver: func(ctx context.Context, networkType string, profile cosmos.NetworkProfileConfig, cfg cosmos.Customization) (string, error) {
			return "https://custom.snapshot.local/latest.tar.lz4", nil
		},
	}))

	got := networkModule.SnapshotURL("mainnet")
	if got != "https://custom.snapshot.local/latest.tar.lz4" {
		t.Fatalf("unexpected snapshot url: %q", got)
	}
}

func TestHooks_GenesisMutatorRuns(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithHooks(cosmos.Hooks{
		AdditionalGenesisMutators: []cosmos.GenesisMutator{markerMutator{}},
	}))

	input := mustMarshalJSON(t, map[string]any{
		"chain_id":   "cosmoshub-4",
		"validators": []any{},
		"app_state": map[string]any{
			"gov":          map[string]any{"params": map[string]any{}},
			"staking":      map[string]any{"params": map[string]any{}, "pool": map[string]any{}},
			"slashing":     map[string]any{},
			"distribution": map[string]any{},
			"auth":         map[string]any{"accounts": []any{}},
			"bank":         map[string]any{"balances": []any{}, "supply": []any{}},
			"genutil":      map[string]any{"gen_txs": []any{}},
		},
	})

	opts := network.GenesisOptions{
		ChainID: "cosmosdevnet-1",
		Validators: []network.ValidatorInfo{
			{
				Moniker:         "validator-0",
				ConsPubKey:      "dGVzdC1wdWJrZXk=",
				OperatorAddress: "cosmosvaloper1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqq8tx58h",
				SelfDelegation:  "1000000",
			},
		},
	}

	out, err := networkModule.ModifyGenesis(input, opts)
	if err != nil {
		t.Fatalf("ModifyGenesis returned error: %v", err)
	}

	var gen map[string]any
	if err := json.Unmarshal(out, &gen); err != nil {
		t.Fatalf("failed to unmarshal output genesis: %v", err)
	}

	appState, _ := asMap(gen["app_state"])
	gov, _ := asMap(appState["gov"])
	if gov["marker"] != "hooked" {
		t.Fatalf("expected gov marker injected by mutator, got %v", gov["marker"])
	}
}

func TestHooks_RPCRequestMiddlewareRuns(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithHooks(cosmos.Hooks{
		RPCRequestMiddleware: func(ctx context.Context, endpoint string, out any, next cosmos.JSONRequestFunc) error {
			return fmt.Errorf("blocked by middleware: %s", endpoint)
		},
	}))

	resp, err := networkModule.GetBlockHeight(context.Background(), "http://localhost:26657")
	if err != nil {
		t.Fatalf("GetBlockHeight returned error: %v", err)
	}
	if resp == nil || !strings.Contains(resp.Error, "blocked by middleware") {
		t.Fatalf("expected middleware error response, got %+v", resp)
	}
}
