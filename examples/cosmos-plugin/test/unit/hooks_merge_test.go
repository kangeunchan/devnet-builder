package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

type namedMarkerMutator struct {
	name   string
	marker string
}

func (m namedMarkerMutator) Name() string { return m.name }

func (m namedMarkerMutator) MutateModule(moduleName string, module map[string]any, _ network.GenesisOptions, _ network.GenesisConfig) error {
	if moduleName != "gov" {
		return nil
	}
	module["marker"] = m.marker
	return nil
}

func TestHooks_MergePreservesExistingFields(t *testing.T) {
	networkModule := cosmos.New(
		cosmos.WithHooks(cosmos.Hooks{
			SnapshotURLResolver: func(ctx context.Context, networkType string, profile cosmos.NetworkProfileConfig, cfg cosmos.Customization) (string, error) {
				_ = ctx
				_ = networkType
				_ = profile
				_ = cfg
				return "https://snapshot.from.first-hook/snapshot.tar.lz4", nil
			},
		}),
		cosmos.WithHooks(cosmos.Hooks{
			RPCRequestMiddleware: func(ctx context.Context, endpoint string, out any, next cosmos.JSONRequestFunc) error {
				_ = ctx
				_ = out
				_ = next
				return fmt.Errorf("rpc blocked by middleware: %s", endpoint)
			},
		}),
	)

	if got := networkModule.SnapshotURL("mainnet"); got != "https://snapshot.from.first-hook/snapshot.tar.lz4" {
		t.Fatalf("SnapshotURL(mainnet) = %q", got)
	}

	resp, err := networkModule.GetBlockHeight(context.Background(), "http://localhost:26657")
	if err != nil {
		t.Fatalf("GetBlockHeight returned error: %v", err)
	}
	if resp == nil || resp.Error == "" {
		t.Fatalf("expected middleware error response, got %+v", resp)
	}
}

func TestHooks_MergeReplacesAdditionalMutatorsWithLatestNonEmpty(t *testing.T) {
	networkModule := cosmos.New(
		cosmos.WithHooks(cosmos.Hooks{
			AdditionalGenesisMutators: []cosmos.GenesisMutator{
				namedMarkerMutator{name: "first", marker: "first"},
			},
		}),
		cosmos.WithHooks(cosmos.Hooks{
			AdditionalGenesisMutators: []cosmos.GenesisMutator{
				namedMarkerMutator{name: "second", marker: "second"},
			},
		}),
	)

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
	if got := gov["marker"]; got != "second" {
		t.Fatalf("expected latest mutator marker, got %v", got)
	}
}
