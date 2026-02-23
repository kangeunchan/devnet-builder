package devnet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

type Runner func(context.Context, string, ...string) ([]byte, error)

type GenerateConfig struct {
	Runner              Runner
	Binary              string
	Config              network.GeneratorConfig
	Defaults            network.GeneratorConfig
	TemplateGenesisPath string
}

type validatorArtifacts struct {
	homes     []string
	addresses []string
	gentx     [][]string
}

func Generate(ctx context.Context, cfg GenerateConfig) error {
	ctx = ensureContext(ctx)

	normalized, err := normalizeGeneratorConfig(cfg.Config, cfg.Defaults)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(normalized.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", normalized.OutputDir, err)
	}

	templateGenesis, err := loadTemplateGenesis(cfg.TemplateGenesisPath)
	if err != nil {
		return err
	}

	artifacts, err := buildValidatorArtifacts(ctx, cfg.Runner, cfg.Binary, normalized, templateGenesis)
	if err != nil {
		return err
	}

	firstHome := artifacts.homes[0]

	if err := addRemainingValidatorAccounts(ctx, cfg.Runner, cfg.Binary, firstHome, normalized.ValidatorBalance, artifacts.addresses); err != nil {
		return err
	}

	if err := addAdditionalAccounts(ctx, cfg.Runner, cfg.Binary, firstHome, normalized.NumAccounts, normalized.AccountBalance); err != nil {
		return err
	}

	if err := collectGentxs(ctx, cfg.Runner, cfg.Binary, firstHome, artifacts.gentx); err != nil {
		return err
	}

	if err := writeFinalGenesis(normalized.OutputDir, firstHome); err != nil {
		return err
	}

	return nil
}

func normalizeGeneratorConfig(config, defaults network.GeneratorConfig) (network.GeneratorConfig, error) {
	cfg := config

	if cfg.NumValidators <= 0 {
		return cfg, fmt.Errorf("num_validators must be > 0")
	}
	if cfg.ChainID == "" {
		cfg.ChainID = defaults.ChainID
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = defaults.OutputDir
	}
	if cfg.ValidatorBalance == "" {
		cfg.ValidatorBalance = defaults.ValidatorBalance
	}
	if cfg.ValidatorStake == "" {
		cfg.ValidatorStake = defaults.ValidatorStake
	}
	if cfg.AccountBalance == "" {
		cfg.AccountBalance = defaults.AccountBalance
	}

	return cfg, nil
}

func loadTemplateGenesis(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template genesis %q: %w", path, err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("template genesis %q is empty", path)
	}

	return content, nil
}

func buildValidatorArtifacts(
	ctx context.Context,
	run Runner,
	binary string,
	cfg network.GeneratorConfig,
	templateGenesis []byte,
) (*validatorArtifacts, error) {
	artifacts := &validatorArtifacts{
		homes:     make([]string, 0, cfg.NumValidators),
		addresses: make([]string, 0, cfg.NumValidators),
		gentx:     make([][]string, cfg.NumValidators),
	}

	for i := 0; i < cfg.NumValidators; i++ {
		home := filepath.Join(cfg.OutputDir, fmt.Sprintf("validator%d", i))
		moniker := fmt.Sprintf("validator%d", i)
		keyName := moniker

		if err := os.MkdirAll(home, 0o755); err != nil {
			return nil, fmt.Errorf("create validator home %q: %w", home, err)
		}

		if _, err := run(ctx, binary, "init", moniker, "--chain-id", cfg.ChainID, "--home", home, "--overwrite"); err != nil {
			return nil, fmt.Errorf("init %s: %w", moniker, err)
		}

		if len(templateGenesis) > 0 {
			genesisPath := filepath.Join(home, "config", "genesis.json")
			if err := os.WriteFile(genesisPath, templateGenesis, 0o644); err != nil {
				return nil, fmt.Errorf("write template genesis for %s: %w", moniker, err)
			}
		}

		address, err := createKeyAndAddress(ctx, run, binary, home, keyName)
		if err != nil {
			return nil, fmt.Errorf("create validator key %s: %w", keyName, err)
		}

		if _, err := run(ctx, binary, "genesis", "add-genesis-account", address, cfg.ValidatorBalance, "--home", home); err != nil {
			return nil, fmt.Errorf("add genesis account for %s (%s): %w", keyName, address, err)
		}

		if _, err := run(ctx, binary, "genesis", "gentx", keyName, cfg.ValidatorStake, "--chain-id", cfg.ChainID, "--keyring-backend", "test", "--home", home); err != nil {
			return nil, fmt.Errorf("gentx for %s: %w", keyName, err)
		}

		gentxFiles, err := filepath.Glob(filepath.Join(home, "config", "gentx", "*.json"))
		if err != nil {
			return nil, fmt.Errorf("list gentx files for %s: %w", keyName, err)
		}
		if len(gentxFiles) == 0 {
			return nil, fmt.Errorf("no gentx file found for %s", keyName)
		}

		artifacts.homes = append(artifacts.homes, home)
		artifacts.addresses = append(artifacts.addresses, address)
		artifacts.gentx[i] = gentxFiles
	}

	return artifacts, nil
}

func addRemainingValidatorAccounts(
	ctx context.Context,
	run Runner,
	binary string,
	firstHome string,
	validatorBalance string,
	addresses []string,
) error {
	if len(addresses) <= 1 {
		return nil
	}

	for i := 1; i < len(addresses); i++ {
		address := addresses[i]
		if _, err := run(ctx, binary, "genesis", "add-genesis-account", address, validatorBalance, "--home", firstHome); err != nil {
			return fmt.Errorf("add validator%d account to collect home: %w", i, err)
		}
	}

	return nil
}

func addAdditionalAccounts(
	ctx context.Context,
	run Runner,
	binary string,
	home string,
	numAccounts int,
	accountBalance string,
) error {
	if numAccounts <= 0 {
		return nil
	}

	for i := 0; i < numAccounts; i++ {
		keyName := fmt.Sprintf("account%d", i)
		address, err := createKeyAndAddress(ctx, run, binary, home, keyName)
		if err != nil {
			return fmt.Errorf("create additional account %s: %w", keyName, err)
		}

		if _, err := run(ctx, binary, "genesis", "add-genesis-account", address, accountBalance, "--home", home); err != nil {
			return fmt.Errorf("fund additional account %s (%s): %w", keyName, address, err)
		}
	}

	return nil
}

func collectGentxs(
	ctx context.Context,
	run Runner,
	binary string,
	firstHome string,
	gentxFiles [][]string,
) error {
	collectDir := filepath.Join(firstHome, "config", "gentx")
	if err := os.MkdirAll(collectDir, 0o755); err != nil {
		return fmt.Errorf("create collect gentx dir %q: %w", collectDir, err)
	}

	for i := 1; i < len(gentxFiles); i++ {
		for _, src := range gentxFiles[i] {
			base := filepath.Base(src)
			dst := filepath.Join(collectDir, fmt.Sprintf("v%d_%s", i, base))
			if err := CopyFile(src, dst); err != nil {
				return fmt.Errorf("copy gentx %q to %q: %w", src, dst, err)
			}
		}
	}

	if _, err := run(ctx, binary, "genesis", "collect-gentxs", "--home", firstHome); err != nil {
		return fmt.Errorf("collect-gentxs failed: %w", err)
	}
	if _, err := run(ctx, binary, "genesis", "validate", "--home", firstHome); err != nil {
		return fmt.Errorf("genesis validate failed: %w", err)
	}

	return nil
}

func writeFinalGenesis(outputDir, firstHome string) error {
	sourcePath := filepath.Join(firstHome, "config", "genesis.json")
	finalGenesis, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read final genesis %q: %w", sourcePath, err)
	}

	outputPath := filepath.Join(outputDir, "genesis.json")
	if err := os.WriteFile(outputPath, finalGenesis, 0o644); err != nil {
		return fmt.Errorf("write final genesis %q: %w", outputPath, err)
	}

	return nil
}

func createKeyAndAddress(
	ctx context.Context,
	run Runner,
	binary string,
	home string,
	keyName string,
) (string, error) {
	out, err := run(ctx, binary, "keys", "add", keyName, "--keyring-backend", "test", "--home", home, "--output", "json")
	if err != nil {
		return "", fmt.Errorf("keys add %s: %w", keyName, err)
	}

	address, err := ParseKeyAddress(out)
	if err != nil {
		return "", fmt.Errorf("parse key output for %s: %w", keyName, err)
	}

	return address, nil
}

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
