package cosmos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

type commandRunnerFunc func(context.Context, string, ...string) ([]byte, error)

// ============================================
// Devnet generator
// ============================================

// GenerateDevnet builds a local validator set and writes the resulting genesis
// to <output_dir>/genesis.json. genesisFile is treated as an optional template
// input path only.

func (n *CosmosNetwork) GenerateDevnet(ctx context.Context, config network.GeneratorConfig, genesisFile string) error {
	ctx = ensureContext(ctx)
	run := n.commandRunner()
	defaultConfig := n.DefaultGeneratorConfig()

	if config.NumValidators <= 0 {
		return fmt.Errorf("num_validators must be > 0")
	}
	if config.ChainID == "" {
		config.ChainID = defaultConfig.ChainID
	}
	if config.OutputDir == "" {
		config.OutputDir = defaultConfig.OutputDir
	}
	if config.ValidatorBalance == "" {
		config.ValidatorBalance = defaultConfig.ValidatorBalance
	}
	if config.ValidatorStake == "" {
		config.ValidatorStake = defaultConfig.ValidatorStake
	}
	if config.AccountBalance == "" {
		config.AccountBalance = defaultConfig.AccountBalance
	}

	if err := os.MkdirAll(config.OutputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output dir %q: %w", config.OutputDir, err)
	}

	var templateGenesis []byte
	if genesisFile != "" {
		b, err := os.ReadFile(genesisFile)
		if err != nil {
			return fmt.Errorf("failed to read template genesis %q: %w", genesisFile, err)
		}
		if len(b) == 0 {
			return fmt.Errorf("template genesis %q is empty", genesisFile)
		}
		templateGenesis = b
	}

	binary := n.BinaryName()
	nodeHomes := make([]string, 0, config.NumValidators)
	gentxFiles := make([][]string, config.NumValidators)
	validatorAddresses := make([]string, 0, config.NumValidators)

	for i := 0; i < config.NumValidators; i++ {
		home := filepath.Join(config.OutputDir, fmt.Sprintf("validator%d", i))
		nodeHomes = append(nodeHomes, home)
		if err := os.MkdirAll(home, 0o755); err != nil {
			return fmt.Errorf("failed to create validator home %q: %w", home, err)
		}

		moniker := fmt.Sprintf("validator%d", i)
		if _, err := run(ctx, binary, "init", moniker, "--chain-id", config.ChainID, "--home", home, "--overwrite"); err != nil {
			return fmt.Errorf("init failed for %s: %w", moniker, err)
		}

		if len(templateGenesis) > 0 {
			gpath := filepath.Join(home, "config", "genesis.json")
			if err := os.WriteFile(gpath, templateGenesis, 0o644); err != nil {
				return fmt.Errorf("failed to write template genesis for %s: %w", moniker, err)
			}
		}

		keyName := fmt.Sprintf("validator%d", i)
		keyOut, err := run(ctx, binary, "keys", "add", keyName,
			"--keyring-backend", "test",
			"--home", home,
			"--output", "json")
		if err != nil {
			return fmt.Errorf("keys add failed for %s: %w", keyName, err)
		}

		addr, err := parseKeyAddress(keyOut)
		if err != nil {
			return fmt.Errorf("failed to parse key address for %s: %w", keyName, err)
		}
		validatorAddresses = append(validatorAddresses, addr)

		if _, err := run(ctx, binary, "genesis", "add-genesis-account", addr, config.ValidatorBalance, "--home", home); err != nil {
			return fmt.Errorf("add-genesis-account failed for %s (%s): %w", keyName, addr, err)
		}

		if _, err := run(ctx, binary, "genesis", "gentx", keyName, config.ValidatorStake,
			"--chain-id", config.ChainID,
			"--keyring-backend", "test",
			"--home", home); err != nil {
			return fmt.Errorf("gentx failed for %s: %w", keyName, err)
		}

		files, err := filepath.Glob(filepath.Join(home, "config", "gentx", "*.json"))
		if err != nil {
			return fmt.Errorf("failed to list gentx files for %s: %w", keyName, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("no gentx file found for %s", keyName)
		}
		gentxFiles[i] = files
	}

	firstHome := nodeHomes[0]

	for i := 1; i < len(validatorAddresses); i++ {
		addr := validatorAddresses[i]
		if _, err := run(ctx, binary, "genesis", "add-genesis-account", addr, config.ValidatorBalance, "--home", firstHome); err != nil {
			return fmt.Errorf("add-genesis-account failed in collect home for validator%d (%s): %w", i, addr, err)
		}
	}

	for i := 0; i < config.NumAccounts; i++ {
		keyName := fmt.Sprintf("account%d", i)
		keyOut, err := run(ctx, binary, "keys", "add", keyName,
			"--keyring-backend", "test",
			"--home", firstHome,
			"--output", "json")
		if err != nil {
			return fmt.Errorf("keys add failed for %s: %w", keyName, err)
		}
		addr, err := parseKeyAddress(keyOut)
		if err != nil {
			return fmt.Errorf("failed to parse key address for %s: %w", keyName, err)
		}
		if _, err := run(ctx, binary, "genesis", "add-genesis-account", addr, config.AccountBalance, "--home", firstHome); err != nil {
			return fmt.Errorf("add-genesis-account failed for %s (%s): %w", keyName, addr, err)
		}
	}

	collectGentxDir := filepath.Join(firstHome, "config", "gentx")
	if err := os.MkdirAll(collectGentxDir, 0o755); err != nil {
		return fmt.Errorf("failed to create collect gentx dir: %w", err)
	}

	for i := 1; i < len(gentxFiles); i++ {
		for _, src := range gentxFiles[i] {
			base := filepath.Base(src)
			dst := filepath.Join(collectGentxDir, fmt.Sprintf("v%d_%s", i, base))
			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("failed to copy gentx %q to %q: %w", src, dst, err)
			}
		}
	}

	if _, err := run(ctx, binary, "genesis", "collect-gentxs", "--home", firstHome); err != nil {
		return fmt.Errorf("collect-gentxs failed: %w", err)
	}

	if _, err := run(ctx, binary, "genesis", "validate", "--home", firstHome); err != nil {
		return fmt.Errorf("genesis validate failed: %w", err)
	}

	finalGenesisPath := filepath.Join(firstHome, "config", "genesis.json")
	finalGenesis, err := os.ReadFile(finalGenesisPath)
	if err != nil {
		return fmt.Errorf("failed to read final genesis: %w", err)
	}

	outPath := filepath.Join(config.OutputDir, "genesis.json")
	if err := os.WriteFile(outPath, finalGenesis, 0o644); err != nil {
		return fmt.Errorf("failed to write final genesis to %q: %w", outPath, err)
	}

	return nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx = ensureContext(ctx)

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("command failed: %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return out, nil
}

func (n *CosmosNetwork) commandRunner() commandRunnerFunc {
	if n != nil && n.runCmd != nil {
		return n.runCmd
	}
	return runCommand
}

func parseKeyAddress(out []byte) (string, error) {
	var resp struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", err
	}
	if resp.Address == "" {
		return "", fmt.Errorf("empty address in key output")
	}
	return resp.Address, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}

	return out.Close()
}
