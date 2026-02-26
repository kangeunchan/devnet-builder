package manage

import (
	"strings"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/config"
	"github.com/altuslabsxyz/devnet-builder/types"
	"github.com/spf13/cobra"
)

type deployFlagSnapshot struct {
	deployNetwork           string
	deployBlockchainNetwork string
	deployValidators        int
	deployMode              string
	deployStableVersion     string
	deployNoCache           bool
	deployAccounts          int
	deployNoInteractive     bool
	deployBinary            string
	deployFork              bool
	deployTestMnemonic      bool
}

func snapshotDeployFlags() deployFlagSnapshot {
	return deployFlagSnapshot{
		deployNetwork:           deployNetwork,
		deployBlockchainNetwork: deployBlockchainNetwork,
		deployValidators:        deployValidators,
		deployMode:              deployMode,
		deployStableVersion:     deployStableVersion,
		deployNoCache:           deployNoCache,
		deployAccounts:          deployAccounts,
		deployNoInteractive:     deployNoInteractive,
		deployBinary:            deployBinary,
		deployFork:              deployFork,
		deployTestMnemonic:      deployTestMnemonic,
	}
}

func restoreDeployFlags(s deployFlagSnapshot) {
	deployNetwork = s.deployNetwork
	deployBlockchainNetwork = s.deployBlockchainNetwork
	deployValidators = s.deployValidators
	deployMode = s.deployMode
	deployStableVersion = s.deployStableVersion
	deployNoCache = s.deployNoCache
	deployAccounts = s.deployAccounts
	deployNoInteractive = s.deployNoInteractive
	deployBinary = s.deployBinary
	deployFork = s.deployFork
	deployTestMnemonic = s.deployTestMnemonic
}

func newDeployTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringVar(&deployNetwork, "network", "mainnet", "")
	cmd.Flags().StringVar(&deployBlockchainNetwork, "blockchain", "stable", "")
	cmd.Flags().IntVar(&deployValidators, "validators", 4, "")
	cmd.Flags().StringVar(&deployMode, "mode", "docker", "")
	cmd.Flags().StringVar(&deployStableVersion, "network-version", "latest", "")
	cmd.Flags().BoolVar(&deployNoCache, "no-cache", false, "")
	cmd.Flags().IntVar(&deployAccounts, "accounts", 4, "")
	return cmd
}

func TestApplyDeployFlagOverrides(t *testing.T) {
	snapshot := snapshotDeployFlags()
	t.Cleanup(func() { restoreDeployFlags(snapshot) })

	cmd := newDeployTestCmd()
	_ = cmd.Flags().Set("network", "testnet")
	_ = cmd.Flags().Set("blockchain", "ault")
	_ = cmd.Flags().Set("validators", "12")
	_ = cmd.Flags().Set("mode", "docker")
	_ = cmd.Flags().Set("network-version", "v1.2.3")
	_ = cmd.Flags().Set("no-cache", "true")
	_ = cmd.Flags().Set("accounts", "9")

	fileCfg := &config.FileConfig{}
	applyDeployFlagOverrides(cmd, fileCfg)

	if fileCfg.Network == nil || *fileCfg.Network != "testnet" {
		t.Fatalf("expected network override")
	}
	if fileCfg.BlockchainNetwork == nil || *fileCfg.BlockchainNetwork != "ault" {
		t.Fatalf("expected blockchain override")
	}
	if fileCfg.Validators == nil || *fileCfg.Validators != 12 {
		t.Fatalf("expected validators override")
	}
	if fileCfg.ExecutionMode == nil || *fileCfg.ExecutionMode != types.ExecutionModeDocker {
		t.Fatalf("expected mode override")
	}
	if fileCfg.NetworkVersion == nil || *fileCfg.NetworkVersion != "v1.2.3" {
		t.Fatalf("expected network version override")
	}
	if fileCfg.NoCache == nil || *fileCfg.NoCache != true {
		t.Fatalf("expected no-cache override")
	}
	if fileCfg.Accounts == nil || *fileCfg.Accounts != 9 {
		t.Fatalf("expected accounts override")
	}
}

func TestApplyDeployEnvOverrides(t *testing.T) {
	snapshot := snapshotDeployFlags()
	t.Cleanup(func() { restoreDeployFlags(snapshot) })

	cmd := newDeployTestCmd()
	fileCfg := &config.FileConfig{}

	t.Setenv("DEVNET_NETWORK", "testnet")
	t.Setenv("DEVNET_MODE", "local")
	t.Setenv("DEVNET_NETWORK_VERSION", "v2.1.0")

	applyDeployEnvOverrides(cmd, fileCfg)

	if fileCfg.Network == nil || *fileCfg.Network != "testnet" {
		t.Fatalf("expected network env override")
	}
	if fileCfg.ExecutionMode == nil || *fileCfg.ExecutionMode != types.ExecutionModeLocal {
		t.Fatalf("expected mode env override")
	}
	if fileCfg.NetworkVersion == nil || *fileCfg.NetworkVersion != "v2.1.0" {
		t.Fatalf("expected network version env override")
	}
}

func TestExtractDeployResolvedConfig(t *testing.T) {
	snapshot := snapshotDeployFlags()
	t.Cleanup(func() { restoreDeployFlags(snapshot) })

	network := "mainnet"
	blockchain := "stable"
	validators := 4
	mode := types.ExecutionModeDocker
	version := "v1.0.0"
	noCache := true
	accounts := 7

	effectiveCfg := &config.FileConfig{
		Network:           &network,
		BlockchainNetwork: &blockchain,
		Validators:        &validators,
		ExecutionMode:     &mode,
		NetworkVersion:    &version,
		NoCache:           &noCache,
		Accounts:          &accounts,
	}

	deployNoInteractive = false
	deployBinary = ""
	resolved := extractDeployResolvedConfig(effectiveCfg, false)
	if resolved.network != "mainnet" || resolved.blockchainNetwork != "stable" {
		t.Fatalf("unexpected resolved network values")
	}
	if resolved.mode != "docker" {
		t.Fatalf("unexpected resolved mode: %q", resolved.mode)
	}
	if !resolved.isInteractive {
		t.Fatalf("expected interactive mode true")
	}
}

func TestValidateDeployConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *deployResolvedConfig
		wantErr bool
	}{
		{
			name:    "invalid network",
			cfg:     &deployResolvedConfig{network: "bad", mode: "docker", validators: 4},
			wantErr: true,
		},
		{
			name:    "invalid docker validators",
			cfg:     &deployResolvedConfig{network: "mainnet", mode: "docker", validators: 101},
			wantErr: true,
		},
		{
			name:    "invalid local validators",
			cfg:     &deployResolvedConfig{network: "mainnet", mode: "local", validators: 5},
			wantErr: true,
		},
		{
			name:    "invalid mode",
			cfg:     &deployResolvedConfig{network: "mainnet", mode: "invalid", validators: 1},
			wantErr: true,
		},
		{
			name:    "valid docker",
			cfg:     &deployResolvedConfig{network: "mainnet", mode: "docker", validators: 10},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeployConfiguration(tt.cfg)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestShouldRunDeployInteractiveSelection(t *testing.T) {
	snapshot := snapshotDeployFlags()
	t.Cleanup(func() { restoreDeployFlags(snapshot) })

	deployNoInteractive = false
	deployBinary = ""
	if !shouldRunDeployInteractiveSelection(false) {
		t.Fatalf("expected interactive selection to run")
	}
	if shouldRunDeployInteractiveSelection(true) {
		t.Fatalf("expected interactive selection disabled in json mode")
	}

	deployNoInteractive = true
	if shouldRunDeployInteractiveSelection(false) {
		t.Fatalf("expected interactive selection disabled with --no-interactive")
	}
}

func TestValidateDeprecatedDeployBinaryFlag(t *testing.T) {
	snapshot := snapshotDeployFlags()
	t.Cleanup(func() { restoreDeployFlags(snapshot) })

	deployBinary = ""
	if err := validateDeprecatedDeployBinaryFlag(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	deployBinary = "/tmp/custom"
	err := validateDeprecatedDeployBinaryFlag()
	if err == nil {
		t.Fatalf("expected deprecated binary flag error")
	}
	if !strings.Contains(err.Error(), "--binary flag has been removed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestApplyDeployEnvOverrides_DoesNotOverrideChangedFlags(t *testing.T) {
	snapshot := snapshotDeployFlags()
	t.Cleanup(func() { restoreDeployFlags(snapshot) })

	cmd := newDeployTestCmd()
	_ = cmd.Flags().Set("network", "mainnet")

	fileCfg := &config.FileConfig{}
	t.Setenv("DEVNET_NETWORK", "testnet")
	applyDeployEnvOverrides(cmd, fileCfg)

	if fileCfg.Network != nil {
		t.Fatalf("expected network env not to override changed flag")
	}
}
