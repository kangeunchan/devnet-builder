package unit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

func TestGenerateDevnet_ValidatesNumValidators(t *testing.T) {
	networkModule := cosmos.New()

	err := networkModule.GenerateDevnet(context.Background(), network.GeneratorConfig{NumValidators: 0}, "")
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "num_validators") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateDevnet_ReturnsValidateError(t *testing.T) {
	outputDir := t.TempDir()

	networkModule := cosmos.New(cosmos.WithCommandRunner(newFakeCommandRunner(t, fakeRunnerOptions{
		validateFail: true,
	})))

	err := networkModule.GenerateDevnet(context.Background(), network.GeneratorConfig{
		NumValidators: 1,
		NumAccounts:   0,
		OutputDir:     outputDir,
		ChainID:       "test-chain",
	}, "")
	if err == nil {
		t.Fatalf("expected validate error")
	}
	if !strings.Contains(err.Error(), "genesis validate failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateDevnet_AddsValidatorAccountsToCollectHome(t *testing.T) {
	outputDir := t.TempDir()
	firstHome := filepath.Join(outputDir, "validator0")
	secondAddr := "cosmos1validator1"

	type call struct {
		args []string
	}
	var addAccountCalls []call

	networkModule := cosmos.New(cosmos.WithCommandRunner(newFakeCommandRunner(t, fakeRunnerOptions{
		onAddGenesisAccount: func(args []string) {
			addAccountCalls = append(addAccountCalls, call{args: append([]string(nil), args...)})
		},
	})))

	err := networkModule.GenerateDevnet(context.Background(), network.GeneratorConfig{
		NumValidators: 2,
		NumAccounts:   0,
		OutputDir:     outputDir,
		ChainID:       "test-chain",
	}, "")
	if err != nil {
		t.Fatalf("GenerateDevnet returned error: %v", err)
	}

	var found bool
	for _, c := range addAccountCalls {
		if len(c.args) < 6 {
			continue
		}
		addr := c.args[2]
		home := argValue(c.args, "--home")
		if addr == secondAddr && home == firstHome {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected second validator account to be added to first home %q", firstHome)
	}
}

func TestGenerateDevnet_UsesGenesisFileAsTemplateInputOnly(t *testing.T) {
	outputDir := t.TempDir()
	templatePath := filepath.Join(t.TempDir(), "template.json")
	template := []byte(`{"chain_id":"template-chain","app_state":{"bank":{}}}`)
	if err := os.WriteFile(templatePath, template, 0o644); err != nil {
		t.Fatalf("failed to write template genesis: %v", err)
	}

	networkModule := cosmos.New(cosmos.WithCommandRunner(newFakeCommandRunner(t, fakeRunnerOptions{})))

	err := networkModule.GenerateDevnet(context.Background(), network.GeneratorConfig{
		NumValidators: 1,
		NumAccounts:   0,
		OutputDir:     outputDir,
		ChainID:       "test-chain",
	}, templatePath)
	if err != nil {
		t.Fatalf("GenerateDevnet returned error: %v", err)
	}

	gotTemplate, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("failed to read template genesis after generation: %v", err)
	}
	if string(gotTemplate) != string(template) {
		t.Fatalf("template genesis should remain unchanged")
	}

	outPath := filepath.Join(outputDir, "genesis.json")
	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read generated genesis output: %v", err)
	}
	if string(outBytes) != string(template) {
		t.Fatalf("generated genesis should come from template input")
	}
}

type fakeRunnerOptions struct {
	validateFail        bool
	onAddGenesisAccount func(args []string)
}

func newFakeCommandRunner(t *testing.T, opts fakeRunnerOptions) func(context.Context, string, ...string) ([]byte, error) {
	t.Helper()

	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		home := argValue(args, "--home")

		if len(args) >= 2 && args[0] == "init" {
			if home == "" {
				return nil, fmt.Errorf("missing --home")
			}
			if err := os.MkdirAll(filepath.Join(home, "config", "gentx"), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(home, "config", "genesis.json"), []byte(`{"chain_id":"test-chain","app_state":{}}`), 0o644); err != nil {
				return nil, err
			}
			return []byte("ok"), nil
		}

		if len(args) >= 3 && args[0] == "keys" && args[1] == "add" {
			keyName := args[2]
			return []byte(fmt.Sprintf(`{"address":"cosmos1%s"}`, keyName)), nil
		}

		if len(args) >= 3 && args[0] == "genesis" && args[1] == "add-genesis-account" {
			if opts.onAddGenesisAccount != nil {
				opts.onAddGenesisAccount(args)
			}
			return []byte("ok"), nil
		}

		if len(args) >= 3 && args[0] == "genesis" && args[1] == "gentx" {
			if home == "" {
				return nil, fmt.Errorf("missing --home")
			}
			gentxPath := filepath.Join(home, "config", "gentx", "gentx.json")
			if err := os.WriteFile(gentxPath, []byte(`{}`), 0o644); err != nil {
				return nil, err
			}
			return []byte("ok"), nil
		}

		if len(args) >= 2 && args[0] == "genesis" && args[1] == "validate" {
			if opts.validateFail {
				return nil, fmt.Errorf("invalid genesis")
			}
			return []byte("ok"), nil
		}

		return []byte("ok"), nil
	}
}

func argValue(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}
