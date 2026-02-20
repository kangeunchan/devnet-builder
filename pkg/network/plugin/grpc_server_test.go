package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

type fallbackOnlyModule struct {
	network.Module
	lastOpts network.GenesisOptions
}

func (m *fallbackOnlyModule) ModifyGenesis(genesis []byte, opts network.GenesisOptions) ([]byte, error) {
	m.lastOpts = opts

	var doc map[string]any
	if err := json.Unmarshal(genesis, &doc); err != nil {
		return nil, err
	}

	doc["chain_id"] = opts.ChainID
	doc["validator_count"] = len(opts.Validators)

	accounts := make([]map[string]string, 0, len(opts.AddAccounts))
	for _, account := range opts.AddAccounts {
		accounts = append(accounts, map[string]string{
			"name":    account.Name,
			"address": account.Address,
			"balance": account.Balance,
		})
	}
	doc["add_accounts"] = accounts

	return json.Marshal(doc)
}

type directFileModule struct {
	*fallbackOnlyModule
}

func (m *directFileModule) ModifyGenesisFile(inputPath, outputPath string, opts network.GenesisOptions) (int64, error) {
	m.lastOpts = opts

	genesis, err := os.ReadFile(inputPath)
	if err != nil {
		return 0, err
	}

	modified, err := m.ModifyGenesis(genesis, opts)
	if err != nil {
		return 0, err
	}

	if err := os.WriteFile(outputPath, modified, 0o644); err != nil {
		return 0, err
	}

	return int64(len(modified)), nil
}

func TestGRPCServer_ModifyGenesisFile_FallbackMatchesDirect(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.json")
	fallbackOutputPath := filepath.Join(tmpDir, "fallback-output.json")
	directOutputPath := filepath.Join(tmpDir, "direct-output.json")

	inputGenesis := []byte(`{"chain_id":"old-chain","app_state":{"bank":{"balances":[]}}}`)
	if err := os.WriteFile(inputPath, inputGenesis, 0o644); err != nil {
		t.Fatalf("failed to write input genesis: %v", err)
	}

	fallbackReq := newModifyGenesisFileRequest(inputPath, fallbackOutputPath)
	directReq := newModifyGenesisFileRequest(inputPath, directOutputPath)

	fallbackImpl := &fallbackOnlyModule{}
	directImpl := &directFileModule{fallbackOnlyModule: &fallbackOnlyModule{}}

	fallbackServer := NewGRPCServer(fallbackImpl)
	directServer := NewGRPCServer(directImpl)

	fallbackResp, err := fallbackServer.ModifyGenesisFile(context.Background(), fallbackReq)
	if err != nil {
		t.Fatalf("fallback ModifyGenesisFile returned error: %v", err)
	}
	if fallbackResp.Error != "" {
		t.Fatalf("fallback ModifyGenesisFile response error: %s", fallbackResp.Error)
	}

	directResp, err := directServer.ModifyGenesisFile(context.Background(), directReq)
	if err != nil {
		t.Fatalf("direct ModifyGenesisFile returned error: %v", err)
	}
	if directResp.Error != "" {
		t.Fatalf("direct ModifyGenesisFile response error: %s", directResp.Error)
	}

	fallbackOut, err := os.ReadFile(fallbackOutputPath)
	if err != nil {
		t.Fatalf("failed to read fallback output: %v", err)
	}
	directOut, err := os.ReadFile(directOutputPath)
	if err != nil {
		t.Fatalf("failed to read direct output: %v", err)
	}

	var fallbackDoc map[string]any
	if err := json.Unmarshal(fallbackOut, &fallbackDoc); err != nil {
		t.Fatalf("failed to unmarshal fallback output: %v", err)
	}
	var directDoc map[string]any
	if err := json.Unmarshal(directOut, &directDoc); err != nil {
		t.Fatalf("failed to unmarshal direct output: %v", err)
	}

	if !reflect.DeepEqual(fallbackDoc, directDoc) {
		t.Fatalf("fallback and direct outputs differ\nfallback=%s\ndirect=%s", string(fallbackOut), string(directOut))
	}

	assertMappedAccounts(t, fallbackImpl.lastOpts)
	assertMappedAccounts(t, directImpl.lastOpts)
}

func newModifyGenesisFileRequest(inputPath, outputPath string) *ModifyGenesisFileRequest {
	return &ModifyGenesisFileRequest{
		InputPath:     inputPath,
		OutputPath:    outputPath,
		ChainId:       "cosmosdevnet-3",
		NumValidators: 1,
		Validators: []*ValidatorInfo{
			{
				Moniker:         "validator-0",
				ConsPubKey:      "dGVzdC1wdWJrZXk=",
				OperatorAddress: "cosmosvaloper1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqq8tx58h",
				SelfDelegation:  "1000000",
			},
		},
		AddAccounts: []*AccountInfo{
			{
				Name:    "account0",
				Address: "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqps2m9",
				Balance: "2500000uatom",
			},
		},
	}
}

func assertMappedAccounts(t *testing.T, opts network.GenesisOptions) {
	t.Helper()

	if len(opts.AddAccounts) != 1 {
		t.Fatalf("expected 1 mapped account, got %d", len(opts.AddAccounts))
	}
	account := opts.AddAccounts[0]
	if account.Name != "account0" {
		t.Fatalf("unexpected account name: %q", account.Name)
	}
	if account.Address != "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqps2m9" {
		t.Fatalf("unexpected account address: %q", account.Address)
	}
	if account.Balance != "2500000uatom" {
		t.Fatalf("unexpected account balance: %q", account.Balance)
	}
}
