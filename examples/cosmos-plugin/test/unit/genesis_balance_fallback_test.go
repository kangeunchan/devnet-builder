package unit

import (
	"encoding/json"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

func TestModifyGenesis_InvalidAdditionalAccountBalanceUsesDefaultFunding(t *testing.T) {
	networkModule := cosmos.New()

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
		AddAccounts: []network.GenesisAccountInfo{
			{
				Name:    "account0",
				Address: "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqps2m9",
				Balance: "invalid",
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
	bank, _ := asMap(appState["bank"])
	balances, _ := asSlice(bank["balances"])

	// DefaultGeneratorConfig().AccountBalance = 100000000000uatom.
	if !hasBalanceCoinAtLeast(balances, "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqps2m9", "uatom", "100000000000") {
		t.Fatalf("expected invalid balance to fallback to default account funding")
	}
}
