package unit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

func TestModifyGenesis_AppliesCoreDevnetMutations(t *testing.T) {
	networkModule := cosmos.New()

	input := mustMarshalJSON(t, map[string]any{
		"chain_id":   "cosmoshub-4",
		"validators": []any{map[string]any{"address": "legacy"}},
		"app_state": map[string]any{
			"gov": map[string]any{
				"params": map[string]any{
					"voting_period":      "172800s",
					"max_deposit_period": "172800s",
					"deposit_period":     "172800s",
				},
				"proposals": []any{map[string]any{"id": "1"}},
				"votes":     []any{map[string]any{"voter": "legacy"}},
				"deposits":  []any{map[string]any{"proposal_id": "1"}},
			},
			"staking": map[string]any{
				"params": map[string]any{
					"unbonding_time": "1814400s",
					"max_validators": "175",
					"bond_denom":     "uatom",
				},
				"validators":            []any{map[string]any{"operator_address": "legacy"}},
				"delegations":           []any{map[string]any{"delegator_address": "legacy"}},
				"unbonding_delegations": []any{map[string]any{"delegator_address": "legacy"}},
				"redelegations":         []any{map[string]any{"delegator_address": "legacy"}},
				"last_validator_powers": []any{map[string]any{"address": "legacy", "power": "1"}},
				"last_total_power":      "1",
				"pool":                  map[string]any{"bonded_tokens": "1", "not_bonded_tokens": "2"},
			},
			"slashing": map[string]any{
				"signing_infos": []any{map[string]any{"address": "legacy"}},
				"missed_blocks": []any{map[string]any{"address": "legacy"}},
			},
			"distribution": map[string]any{
				"delegator_starting_infos":          []any{map[string]any{"delegator_address": "legacy"}},
				"validator_slash_events":            []any{map[string]any{"validator_address": "legacy"}},
				"outstanding_rewards":               []any{map[string]any{"validator_address": "legacy"}},
				"validator_accumulated_commissions": []any{map[string]any{"validator_address": "legacy"}},
				"validator_historical_rewards":      []any{map[string]any{"validator_address": "legacy"}},
				"validator_current_rewards":         []any{map[string]any{"validator_address": "legacy"}},
			},
			"auth": map[string]any{
				"accounts": []any{
					map[string]any{
						"@type": "/cosmos.auth.v1beta1.ModuleAccount",
						"name":  "bonded_tokens_pool",
						"base_account": map[string]any{
							"address": "cosmos1bonded0000000000000000000000000000",
						},
					},
					map[string]any{
						"@type": "/cosmos.auth.v1beta1.ModuleAccount",
						"name":  "not_bonded_tokens_pool",
						"base_account": map[string]any{
							"address": "cosmos1notbonded0000000000000000000000000",
						},
					},
				},
			},
			"bank": map[string]any{
				"balances": []any{},
				"supply":   []any{},
			},
			"genutil": map[string]any{
				"gen_txs": []any{map[string]any{"type": "legacy"}},
			},
		},
	})

	opts := network.GenesisOptions{
		ChainID: "cosmosdevnet-99",
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
				Balance: "2500000uatom",
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

	if got := gen["chain_id"]; got != "cosmosdevnet-99" {
		t.Fatalf("chain_id not updated: %v", got)
	}

	validators, _ := asSlice(gen["validators"])
	if len(validators) != 0 {
		t.Fatalf("top-level validators should be empty, got %d", len(validators))
	}

	appState, ok := asMap(gen["app_state"])
	if !ok {
		t.Fatalf("app_state missing")
	}

	gov, _ := asMap(appState["gov"])
	govParams, _ := asMap(gov["params"])
	if govParams["voting_period"] != "60s" {
		t.Fatalf("unexpected voting_period: %v", govParams["voting_period"])
	}
	if govParams["max_deposit_period"] != "120s" {
		t.Fatalf("unexpected max_deposit_period: %v", govParams["max_deposit_period"])
	}
	govProposals, _ := asSlice(gov["proposals"])
	if len(govProposals) != 0 {
		t.Fatalf("gov proposals should be cleared, got %d", len(govProposals))
	}

	staking, _ := asMap(appState["staking"])
	stakingParams, _ := asMap(staking["params"])
	if stakingParams["unbonding_time"] != "120s" {
		t.Fatalf("unexpected unbonding_time: %v", stakingParams["unbonding_time"])
	}
	stakingVals, _ := asSlice(staking["validators"])
	if len(stakingVals) != 1 {
		t.Fatalf("staking validators should be rebuilt, got %d", len(stakingVals))
	}
	stakingUndels, _ := asSlice(staking["unbonding_delegations"])
	if len(stakingUndels) != 0 {
		t.Fatalf("unbonding delegations should be cleared, got %d", len(stakingUndels))
	}

	slashing, _ := asMap(appState["slashing"])
	signingInfos, _ := asSlice(slashing["signing_infos"])
	if len(signingInfos) != 0 {
		t.Fatalf("slashing signing_infos should be cleared, got %d", len(signingInfos))
	}

	dist, _ := asMap(appState["distribution"])
	distOutstanding, _ := asSlice(dist["outstanding_rewards"])
	if len(distOutstanding) != 1 {
		t.Fatalf("distribution outstanding rewards should be rebuilt per validator, got %d", len(distOutstanding))
	}

	bank, _ := asMap(appState["bank"])
	balances, _ := asSlice(bank["balances"])
	if !hasBalanceCoinAtLeast(balances, "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqps2m9", "uatom", "2500000") {
		t.Fatalf("bank balances should include funded additional account")
	}
	supply, _ := asSlice(bank["supply"])
	if len(supply) == 0 {
		t.Fatalf("bank supply should be recomputed")
	}

	genutil, _ := asMap(appState["genutil"])
	genTxs, _ := asSlice(genutil["gen_txs"])
	if len(genTxs) != 0 {
		t.Fatalf("genutil gen_txs should be cleared, got %d", len(genTxs))
	}
}

func TestModifyGenesisFile_WritesOutput(t *testing.T) {
	networkModule := cosmos.New()
	tmpDir := t.TempDir()

	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.json")

	input := mustMarshalJSON(t, map[string]any{
		"chain_id":   "cosmoshub-4",
		"validators": []any{},
		"app_state": map[string]any{
			"gov":          map[string]any{"params": map[string]any{}},
			"staking":      map[string]any{"params": map[string]any{}},
			"slashing":     map[string]any{},
			"distribution": map[string]any{},
			"auth":         map[string]any{"accounts": []any{}},
			"bank":         map[string]any{"balances": []any{}, "supply": []any{}},
			"genutil":      map[string]any{"gen_txs": []any{}},
		},
	})

	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatalf("failed to write input genesis: %v", err)
	}

	opts := network.GenesisOptions{
		ChainID: "cosmosdevnet-2",
		Validators: []network.ValidatorInfo{
			{
				Moniker:         "validator-0",
				ConsPubKey:      "dGVzdC1wdWJrZXk=",
				OperatorAddress: "cosmosvaloper1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqq8tx58h",
				SelfDelegation:  "1000000",
			},
		},
	}

	size, err := networkModule.ModifyGenesisFile(inputPath, outputPath, opts)
	if err != nil {
		t.Fatalf("ModifyGenesisFile returned error: %v", err)
	}
	if size <= 0 {
		t.Fatalf("expected positive output size, got %d", size)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output genesis: %v", err)
	}
	if !strings.Contains(string(out), "cosmosdevnet-2") {
		t.Fatalf("output genesis does not contain updated chain id")
	}
}

func TestModifyGenesis_RequiresValidators(t *testing.T) {
	networkModule := cosmos.New()

	input := mustMarshalJSON(t, map[string]any{
		"chain_id": "cosmoshub-4",
		"app_state": map[string]any{
			"gov":          map[string]any{"params": map[string]any{}},
			"staking":      map[string]any{"params": map[string]any{}},
			"slashing":     map[string]any{},
			"distribution": map[string]any{},
			"auth":         map[string]any{"accounts": []any{}},
			"bank":         map[string]any{"balances": []any{}, "supply": []any{}},
			"genutil":      map[string]any{"gen_txs": []any{}},
		},
	})

	_, err := networkModule.ModifyGenesis(input, network.GenesisOptions{ChainID: "cosmosdevnet-2"})
	if err == nil {
		t.Fatalf("expected error when validators are omitted")
	}
	if !strings.Contains(err.Error(), "validators are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestModifyGenesisFile_RequiresValidators(t *testing.T) {
	networkModule := cosmos.New()
	tmpDir := t.TempDir()

	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.json")
	input := mustMarshalJSON(t, map[string]any{
		"chain_id": "cosmoshub-4",
		"app_state": map[string]any{
			"gov":          map[string]any{"params": map[string]any{}},
			"staking":      map[string]any{"params": map[string]any{}},
			"slashing":     map[string]any{},
			"distribution": map[string]any{},
			"auth":         map[string]any{"accounts": []any{}},
			"bank":         map[string]any{"balances": []any{}, "supply": []any{}},
			"genutil":      map[string]any{"gen_txs": []any{}},
		},
	})
	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatalf("failed to write input genesis: %v", err)
	}

	_, err := networkModule.ModifyGenesisFile(inputPath, outputPath, network.GenesisOptions{ChainID: "cosmosdevnet-2"})
	if err == nil {
		t.Fatalf("expected error when validators are omitted")
	}
	if !strings.Contains(err.Error(), "validators are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestModifyGenesis_HandlesModuleOrderIndependently(t *testing.T) {
	networkModule := cosmos.New()

	// Test case: bank appears before auth in genesis (reversed order)
	input := mustMarshalJSON(t, map[string]any{
		"chain_id":   "cosmoshub-4",
		"validators": []any{},
		"app_state": map[string]any{
			"bank": map[string]any{
				"balances": []any{},
				"supply":   []any{},
			},
			"auth": map[string]any{
				"accounts": []any{
					map[string]any{
						"@type": "/cosmos.auth.v1beta1.ModuleAccount",
						"name":  "bonded_tokens_pool",
						"base_account": map[string]any{
							"address": "cosmos1bonded0000000000000000000000000000",
						},
					},
					map[string]any{
						"@type": "/cosmos.auth.v1beta1.ModuleAccount",
						"name":  "not_bonded_tokens_pool",
						"base_account": map[string]any{
							"address": "cosmos1notbonded0000000000000000000000000",
						},
					},
				},
			},
			"staking": map[string]any{
				"params": map[string]any{
					"bond_denom": "uatom",
				},
				"pool": map[string]any{
					"bonded_tokens":     "0",
					"not_bonded_tokens": "0",
				},
			},
			"gov":          map[string]any{"params": map[string]any{}},
			"slashing":     map[string]any{},
			"distribution": map[string]any{},
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
		t.Fatalf("ModifyGenesis failed with bank before auth: %v", err)
	}

	var gen map[string]any
	if err := json.Unmarshal(out, &gen); err != nil {
		t.Fatalf("failed to unmarshal output genesis: %v", err)
	}

	appState, ok := asMap(gen["app_state"])
	if !ok {
		t.Fatalf("app_state missing")
	}

	// Verify bank module was patched correctly despite appearing before auth
	bank, ok := asMap(appState["bank"])
	if !ok {
		t.Fatalf("bank module missing")
	}

	balances, _ := asSlice(bank["balances"])
	if len(balances) < 2 {
		t.Fatalf("expected at least 2 balance entries (bonded/not-bonded pools), got %d", len(balances))
	}

	supply, _ := asSlice(bank["supply"])
	if len(supply) == 0 {
		t.Fatalf("expected supply to be recomputed even when bank appears before auth")
	}

	// Verify module accounts from auth were used in bank patching
	foundBondedPool := false
	for _, bal := range balances {
		balMap, ok := asMap(bal)
		if !ok {
			continue
		}
		addr, _ := balMap["address"].(string)
		if addr == "cosmos1bonded0000000000000000000000000000" {
			foundBondedPool = true
			coins, _ := asSlice(balMap["coins"])
			if len(coins) == 0 {
				t.Fatalf("bonded pool should have coins allocated")
			}
			break
		}
	}

	if !foundBondedPool {
		t.Fatalf("bonded_tokens_pool balance not found - auth module data not used in bank patching")
	}
}

func TestModifyGenesisFile_HandlesModuleOrderIndependently(t *testing.T) {
	networkModule := cosmos.New()
	tmpDir := t.TempDir()

	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.json")

	// Test case: bank appears before auth in genesis (reversed order)
	input := mustMarshalJSON(t, map[string]any{
		"chain_id":   "cosmoshub-4",
		"validators": []any{},
		"app_state": map[string]any{
			"bank": map[string]any{
				"balances": []any{},
				"supply":   []any{},
			},
			"auth": map[string]any{
				"accounts": []any{
					map[string]any{
						"@type": "/cosmos.auth.v1beta1.ModuleAccount",
						"name":  "bonded_tokens_pool",
						"base_account": map[string]any{
							"address": "cosmos1bonded0000000000000000000000000000",
						},
					},
					map[string]any{
						"@type": "/cosmos.auth.v1beta1.ModuleAccount",
						"name":  "not_bonded_tokens_pool",
						"base_account": map[string]any{
							"address": "cosmos1notbonded0000000000000000000000000",
						},
					},
				},
			},
			"staking": map[string]any{
				"params": map[string]any{
					"bond_denom": "uatom",
				},
				"pool": map[string]any{
					"bonded_tokens":     "0",
					"not_bonded_tokens": "0",
				},
			},
			"gov":          map[string]any{"params": map[string]any{}},
			"slashing":     map[string]any{},
			"distribution": map[string]any{},
			"genutil":      map[string]any{"gen_txs": []any{}},
		},
	})

	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatalf("failed to write input genesis: %v", err)
	}

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

	_, err := networkModule.ModifyGenesisFile(inputPath, outputPath, opts)
	if err != nil {
		t.Fatalf("ModifyGenesisFile failed with bank before auth: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output genesis: %v", err)
	}

	var gen map[string]any
	if err := json.Unmarshal(out, &gen); err != nil {
		t.Fatalf("failed to unmarshal output genesis: %v", err)
	}

	appState, ok := asMap(gen["app_state"])
	if !ok {
		t.Fatalf("app_state missing")
	}

	// Verify bank module was patched correctly despite appearing before auth
	bank, ok := asMap(appState["bank"])
	if !ok {
		t.Fatalf("bank module missing")
	}

	balances, _ := asSlice(bank["balances"])
	if len(balances) < 2 {
		t.Fatalf("expected at least 2 balance entries (bonded/not-bonded pools), got %d", len(balances))
	}

	supply, _ := asSlice(bank["supply"])
	if len(supply) == 0 {
		t.Fatalf("expected supply to be recomputed even when bank appears before auth")
	}

	// Verify module accounts from auth were used in bank patching.
	foundBondedPool := false
	for _, bal := range balances {
		balMap, ok := asMap(bal)
		if !ok {
			continue
		}
		addr, _ := balMap["address"].(string)
		if addr == "cosmos1bonded0000000000000000000000000000" {
			foundBondedPool = true
			break
		}
	}

	if !foundBondedPool {
		t.Fatalf("bonded_tokens_pool balance not found - auth module data not used in bank patching")
	}
}

func TestModifyGenesisFile_MatchesInMemoryForAdditionalAccounts(t *testing.T) {
	networkModule := cosmos.New()
	tmpDir := t.TempDir()

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
		ChainID: "cosmosdevnet-3",
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
				Balance: "2500000uatom",
			},
		},
	}

	inMemory, err := networkModule.ModifyGenesis(input, opts)
	if err != nil {
		t.Fatalf("ModifyGenesis returned error: %v", err)
	}

	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.json")
	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatalf("failed to write input genesis: %v", err)
	}

	if _, err := networkModule.ModifyGenesisFile(inputPath, outputPath, opts); err != nil {
		t.Fatalf("ModifyGenesisFile returned error: %v", err)
	}

	fileBased, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output genesis: %v", err)
	}

	var inMemGen map[string]any
	if err := json.Unmarshal(inMemory, &inMemGen); err != nil {
		t.Fatalf("failed to unmarshal in-memory genesis: %v", err)
	}
	var fileGen map[string]any
	if err := json.Unmarshal(fileBased, &fileGen); err != nil {
		t.Fatalf("failed to unmarshal file-based genesis: %v", err)
	}

	inMemApp, _ := asMap(inMemGen["app_state"])
	fileApp, _ := asMap(fileGen["app_state"])
	inMemBank, _ := asMap(inMemApp["bank"])
	fileBank, _ := asMap(fileApp["bank"])

	inMemBalances, _ := asSlice(inMemBank["balances"])
	fileBalances, _ := asSlice(fileBank["balances"])

	if !hasBalanceCoinAtLeast(inMemBalances, "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqps2m9", "uatom", "2500000") {
		t.Fatalf("in-memory genesis is missing expected additional account funding")
	}
	if !hasBalanceCoinAtLeast(fileBalances, "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqps2m9", "uatom", "2500000") {
		t.Fatalf("file-based genesis is missing expected additional account funding")
	}
}

func TestModifyGenesisFile_HandlesLargeGenesisOver50MB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large genesis test in short mode")
	}

	networkModule := cosmos.New()
	tmpDir := t.TempDir()

	inputPath := filepath.Join(tmpDir, "input-large.json")
	outputPath := filepath.Join(tmpDir, "output-large.json")

	largeBlob := strings.Repeat("x", 55*1024*1024)
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
			"large_module": map[string]any{"blob": largeBlob},
		},
	})
	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatalf("failed to write large input genesis: %v", err)
	}

	opts := network.GenesisOptions{
		ChainID: "cosmosdevnet-large",
		Validators: []network.ValidatorInfo{
			{
				Moniker:         "validator-0",
				ConsPubKey:      "dGVzdC1wdWJrZXk=",
				OperatorAddress: "cosmosvaloper1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqq8tx58h",
				SelfDelegation:  "1000000",
			},
		},
	}

	outputSize, err := networkModule.ModifyGenesisFile(inputPath, outputPath, opts)
	if err != nil {
		t.Fatalf("ModifyGenesisFile returned error for large genesis: %v", err)
	}
	if outputSize <= 50*1024*1024 {
		t.Fatalf("expected output larger than 50MB, got %d bytes", outputSize)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read large output genesis: %v", err)
	}
	if !bytes.Contains(out, []byte("cosmosdevnet-large")) {
		t.Fatalf("large output genesis does not contain updated chain id")
	}
}

func hasBalanceCoinAtLeast(balances []any, address, denom, expectedAmount string) bool {
	expected, ok := new(big.Int).SetString(expectedAmount, 10)
	if !ok {
		return false
	}

	for _, raw := range balances {
		balance, ok := asMap(raw)
		if !ok {
			continue
		}
		addr, _ := balance["address"].(string)
		if addr != address {
			continue
		}

		coins, _ := asSlice(balance["coins"])
		for _, coinRaw := range coins {
			coinMap, ok := asMap(coinRaw)
			if !ok {
				continue
			}
			if coinMap["denom"] != denom {
				continue
			}
			amount := strings.TrimSpace(fmt.Sprint(coinMap["amount"]))
			got, ok := new(big.Int).SetString(amount, 10)
			if !ok {
				continue
			}
			return got.Cmp(expected) >= 0
		}
	}

	return false
}
