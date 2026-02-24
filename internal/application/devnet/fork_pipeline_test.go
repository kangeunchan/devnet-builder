package devnet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

func TestNormalizeForkMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: forkModeTrimmed},
		{name: "full", input: "fork-full", want: forkModeFull},
		{name: "trimmed", input: "fork-trimmed", want: forkModeTrimmed},
		{name: "invalid", input: "weird", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeForkMode(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildSnapshotExportOptions_DefaultTrimmedModules(t *testing.T) {
	opts, err := buildSnapshotExportOptions(dto.ProvisionInput{
		ForkMode: forkModeTrimmed,
	}, &ports.ExportOptions{
		ForZeroHeight: false,
	})
	if err != nil {
		t.Fatalf("buildSnapshotExportOptions returned error: %v", err)
	}
	if !opts.ForZeroHeight {
		t.Fatalf("expected ForZeroHeight=true")
	}
	if len(opts.ModulesToExport) == 0 {
		t.Fatalf("expected default trimmed modules to be applied")
	}
	if !slices.Contains(opts.ModulesToExport, "bank") {
		t.Fatalf("expected bank module in export allowlist")
	}
}

func TestBuildSnapshotExportOptions_CustomModules(t *testing.T) {
	opts, err := buildSnapshotExportOptions(dto.ProvisionInput{
		ForkMode:      forkModeFull,
		ExportModules: []string{"auth,bank", "gov"},
	}, nil)
	if err != nil {
		t.Fatalf("buildSnapshotExportOptions returned error: %v", err)
	}
	if !slices.Equal(opts.ModulesToExport, []string{"auth", "bank", "gov"}) {
		t.Fatalf("unexpected modules: %#v", opts.ModulesToExport)
	}
}

func TestBuildSnapshotExportOptions_FullModeWithoutModules(t *testing.T) {
	opts, err := buildSnapshotExportOptions(dto.ProvisionInput{
		ForkMode: forkModeFull,
	}, nil)
	if err != nil {
		t.Fatalf("buildSnapshotExportOptions returned error: %v", err)
	}
	if len(opts.ModulesToExport) != 0 {
		t.Fatalf("expected no module allowlist for fork-full, got %#v", opts.ModulesToExport)
	}
}

func TestNormalizeExportModules_DeduplicatesAndTrims(t *testing.T) {
	got := normalizeExportModules([]string{" auth,bank ", "gov", "bank", "", "gov,staking"})
	want := []string{"auth", "bank", "gov", "staking"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected normalized modules: got=%#v want=%#v", got, want)
	}
}

func TestApplyForkTransform_TrimmedPrunesState(t *testing.T) {
	genesis := map[string]any{
		"chain_id": "cosmoshub-4",
		"app_state": map[string]any{
			"auth": map[string]any{
				"accounts": []any{
					map[string]any{
						"@type": "/cosmos.auth.v1beta1.ModuleAccount",
						"base_account": map[string]any{
							"address": "cosmos1module",
						},
					},
					map[string]any{
						"@type":   "/cosmos.auth.v1beta1.BaseAccount",
						"address": "cosmos1user",
					},
				},
			},
			"bank": map[string]any{
				"balances": []any{
					map[string]any{
						"address": "cosmos1module",
						"coins": []any{
							map[string]any{"denom": "uatom", "amount": "10"},
						},
					},
					map[string]any{
						"address": "cosmos1user",
						"coins": []any{
							map[string]any{"denom": "uatom", "amount": "1000"},
						},
					},
				},
			},
			"gov": map[string]any{
				"proposals": []any{map[string]any{"id": "1"}},
			},
			"staking": map[string]any{
				"validators": []any{map[string]any{"operator_address": "x"}},
			},
			"slashing": map[string]any{
				"signing_infos": []any{map[string]any{"address": "x"}},
			},
			"distribution": map[string]any{
				"validator_historical_rewards": []any{map[string]any{"x": "y"}},
			},
		},
	}

	raw, err := json.Marshal(genesis)
	if err != nil {
		t.Fatalf("marshal genesis: %v", err)
	}

	out, err := applyForkTransform(raw, forkModeTrimmed)
	if err != nil {
		t.Fatalf("applyForkTransform returned error: %v", err)
	}

	decoded, err := decodeJSONMap(out)
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	appState, _ := decoded["app_state"].(map[string]any)
	auth := appState["auth"].(map[string]any)
	bank := appState["bank"].(map[string]any)
	gov := appState["gov"].(map[string]any)

	accounts := auth["accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("expected 1 auth account after trim, got %d", len(accounts))
	}
	balances := bank["balances"].([]any)
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance after trim, got %d", len(balances))
	}
	if len(gov["proposals"].([]any)) != 0 {
		t.Fatalf("expected gov proposals to be cleared")
	}
}

func TestApplyForkTransform_FullModePassthrough(t *testing.T) {
	in := buildBaseGenesisFixture()
	raw, _ := json.Marshal(in)

	out, err := applyForkTransform(raw, forkModeFull)
	if err != nil {
		t.Fatalf("applyForkTransform returned error: %v", err)
	}
	if string(out) != string(raw) {
		t.Fatalf("fork-full should return genesis unchanged")
	}
}

func TestApplyForkTransform_NoModuleAccountsKeepsAuthAndBank(t *testing.T) {
	genesis := buildBaseGenesisFixture()
	auth := genesis["app_state"].(map[string]any)["auth"].(map[string]any)
	auth["accounts"] = []any{
		map[string]any{
			"@type":   "/cosmos.auth.v1beta1.BaseAccount",
			"address": "cosmos1useronly",
		},
	}

	raw, _ := json.Marshal(genesis)
	out, err := applyForkTransform(raw, forkModeTrimmed)
	if err != nil {
		t.Fatalf("applyForkTransform returned error: %v", err)
	}
	decoded, _ := decodeJSONMap(out)
	appState := decoded["app_state"].(map[string]any)
	gotAuthAccounts := appState["auth"].(map[string]any)["accounts"].([]any)
	gotBalances := appState["bank"].(map[string]any)["balances"].([]any)
	if len(gotAuthAccounts) == 0 {
		t.Fatalf("expected auth accounts to be preserved when no module accounts exist")
	}
	if len(gotBalances) == 0 {
		t.Fatalf("expected bank balances to be preserved when no module accounts exist")
	}
}

func TestApplyGenericPatchPolicy(t *testing.T) {
	dir := t.TempDir()
	patchPath := filepath.Join(dir, "patch.json")
	patch := `{
  "module_overrides": {
    "mint": {"params": {"inflation_min":"0.0"}}
  },
  "patches": [
    {"op":"set","path":"/chain_id","value":"cosmosdevnet-1"},
    {"op":"merge","path":"/app_state/staking/params","value":{"unbonding_time":"30s"}},
    {"op":"append","path":"/app_state/gov/proposals","value":{"id":"42"}},
    {"op":"delete","path":"/app_state/slashing/missed_blocks"}
  ]
}`
	if err := os.WriteFile(patchPath, []byte(patch), 0644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	genesis := map[string]any{
		"chain_id": "cosmoshub-4",
		"app_state": map[string]any{
			"staking": map[string]any{"params": map[string]any{"max_validators": "180"}},
			"gov":     map[string]any{"proposals": []any{}},
			"slashing": map[string]any{
				"missed_blocks": []any{"x"},
			},
			"auth": map[string]any{},
			"bank": map[string]any{},
		},
	}
	raw, _ := json.Marshal(genesis)

	out, err := applyGenericPatchPolicy(raw, patchPath)
	if err != nil {
		t.Fatalf("applyGenericPatchPolicy returned error: %v", err)
	}
	decoded, err := decodeJSONMap(out)
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}

	if decoded["chain_id"] != "cosmosdevnet-1" {
		t.Fatalf("expected chain_id override")
	}
	appState := decoded["app_state"].(map[string]any)
	if _, ok := appState["mint"]; !ok {
		t.Fatalf("expected mint module override")
	}
	gov := appState["gov"].(map[string]any)
	if len(gov["proposals"].([]any)) != 1 {
		t.Fatalf("expected appended governance proposal")
	}
	slashing := appState["slashing"].(map[string]any)
	if _, ok := slashing["missed_blocks"]; ok {
		t.Fatalf("expected missed_blocks to be deleted")
	}
}

func TestApplyGenericPatchPolicy_UnknownFieldFails(t *testing.T) {
	dir := t.TempDir()
	patchPath := filepath.Join(dir, "patch.json")
	invalid := `{"unknown_key":true}`
	if err := os.WriteFile(patchPath, []byte(invalid), 0644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	genesis := buildBaseGenesisFixture()
	raw, _ := json.Marshal(genesis)

	_, err := applyGenericPatchPolicy(raw, patchPath)
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "failed to parse genesis patch file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyGenericPatchPolicy_InvalidPointerFails(t *testing.T) {
	dir := t.TempDir()
	patchPath := filepath.Join(dir, "patch.json")
	invalid := `{
  "patches":[
    {"op":"set","path":"app_state/no-leading-slash","value":"x"}
  ]
}`
	if err := os.WriteFile(patchPath, []byte(invalid), 0644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	genesis := buildBaseGenesisFixture()
	raw, _ := json.Marshal(genesis)

	_, err := applyGenericPatchPolicy(raw, patchPath)
	if err == nil {
		t.Fatalf("expected error for invalid pointer")
	}
}

func TestApplyGenericPatchPolicy_InvalidOpFails(t *testing.T) {
	dir := t.TempDir()
	patchPath := filepath.Join(dir, "patch.json")
	invalid := `{
  "patches":[
    {"op":"replace","path":"/chain_id","value":"x"}
  ]
}`
	if err := os.WriteFile(patchPath, []byte(invalid), 0644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	genesis := buildBaseGenesisFixture()
	raw, _ := json.Marshal(genesis)

	_, err := applyGenericPatchPolicy(raw, patchPath)
	if err == nil {
		t.Fatalf("expected error for unsupported op")
	}
	if !strings.Contains(err.Error(), "unsupported patch op") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecomputeDerivedGenesis_RebuildsBankSupply(t *testing.T) {
	genesis := buildBaseGenesisFixture()
	appState := genesis["app_state"].(map[string]any)
	bank := appState["bank"].(map[string]any)
	bank["balances"] = []any{
		map[string]any{
			"address": "cosmos1a",
			"coins": []any{
				map[string]any{"denom": "uatom", "amount": "10"},
				map[string]any{"denom": "uosmo", "amount": "5"},
			},
		},
		map[string]any{
			"address": "cosmos1b",
			"coins": []any{
				map[string]any{"denom": "uatom", "amount": "15"},
			},
		},
	}
	bank["supply"] = []any{
		map[string]any{"denom": "uatom", "amount": "999999"},
	}

	raw, _ := json.Marshal(genesis)
	out, err := recomputeDerivedGenesis(raw)
	if err != nil {
		t.Fatalf("recomputeDerivedGenesis returned error: %v", err)
	}
	decoded, _ := decodeJSONMap(out)
	gotSupply := decoded["app_state"].(map[string]any)["bank"].(map[string]any)["supply"].([]any)
	if len(gotSupply) != 2 {
		t.Fatalf("expected 2 supply denoms, got %d", len(gotSupply))
	}
}

func TestValidateGenesisForDeploy(t *testing.T) {
	genesis := buildBaseGenesisFixture()
	raw, _ := json.Marshal(genesis)
	if err := validateGenesisForDeploy(raw); err != nil {
		t.Fatalf("expected valid genesis, got error: %v", err)
	}

	delete(genesis["app_state"].(map[string]any), "gov")
	raw, _ = json.Marshal(genesis)
	if err := validateGenesisForDeploy(raw); err == nil {
		t.Fatalf("expected missing required module error")
	}
}

func TestEnforceGenesisSizeGuardrailSize(t *testing.T) {
	if err := enforceGenesisSizeGuardrailSize(100, 200); err != nil {
		t.Fatalf("unexpected error for size under limit: %v", err)
	}
	if err := enforceGenesisSizeGuardrailSize(300, 200); err == nil {
		t.Fatalf("expected error for size over limit")
	}
}

func buildBaseGenesisFixture() map[string]any {
	return map[string]any{
		"chain_id": "cosmoshub-4",
		"app_state": map[string]any{
			"auth": map[string]any{
				"accounts": []any{
					map[string]any{
						"@type": "/cosmos.auth.v1beta1.ModuleAccount",
						"base_account": map[string]any{
							"address": "cosmos1module",
						},
					},
					map[string]any{
						"@type":   "/cosmos.auth.v1beta1.BaseAccount",
						"address": "cosmos1user",
					},
				},
			},
			"bank": map[string]any{
				"balances": []any{
					map[string]any{
						"address": "cosmos1module",
						"coins": []any{
							map[string]any{"denom": "uatom", "amount": "10"},
						},
					},
					map[string]any{
						"address": "cosmos1user",
						"coins": []any{
							map[string]any{"denom": "uatom", "amount": "1000"},
						},
					},
				},
			},
			"gov": map[string]any{
				"proposals": []any{map[string]any{"id": "1"}},
			},
			"staking": map[string]any{
				"validators": []any{map[string]any{"operator_address": "x"}},
			},
			"slashing": map[string]any{
				"signing_infos": []any{map[string]any{"address": "x"}},
			},
			"distribution": map[string]any{
				"validator_historical_rewards": []any{map[string]any{"x": "y"}},
			},
			"mint":   map[string]any{},
			"params": map[string]any{},
		},
	}
}
