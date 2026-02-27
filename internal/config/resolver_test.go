package config

import (
	"testing"

	"github.com/altuslabsxyz/devnet-builder/types"
	"github.com/spf13/cobra"
)

func TestResolveRuntimeFileConfigPrecedence(t *testing.T) {
	t.Setenv("DEVNET_NETWORK", "env-network")
	t.Setenv("DEVNET_MODE", "local")
	t.Setenv("DEVNET_NETWORK_VERSION", "v9.9.9")

	cmd := newRuntimeResolverTestCmd()
	if err := cmd.Flags().Set("network", "flag-network"); err != nil {
		t.Fatalf("set network flag: %v", err)
	}
	if err := cmd.Flags().Set("validators", "8"); err != nil {
		t.Fatalf("set validators flag: %v", err)
	}

	mode := types.ExecutionModeDocker
	base := &FileConfig{
		Network:           strPtr("file-network"),
		BlockchainNetwork: strPtr("stable"),
		Validators:        intPtrTest(4),
		ExecutionMode:     &mode,
		NetworkVersion:    strPtr("v1.0.0"),
	}

	resolver := NewResolver()
	resolved := resolver.ResolveRuntimeFileConfig(cmd, base, RuntimeResolveInput{
		Network:           "flag-network",
		BlockchainNetwork: "stable",
		Validators:        8,
		Mode:              types.ExecutionModeDocker,
		NetworkVersion:    "latest",
	})

	if got := derefString(resolved.FileConfig.Network); got != "flag-network" {
		t.Fatalf("network mismatch: got %q", got)
	}
	if got := derefInt(resolved.FileConfig.Validators); got != 8 {
		t.Fatalf("validators mismatch: got %d", got)
	}
	if got := string(derefMode(resolved.FileConfig.ExecutionMode)); got != "local" {
		t.Fatalf("mode mismatch: got %q", got)
	}
	if got := derefString(resolved.FileConfig.NetworkVersion); got != "v9.9.9" {
		t.Fatalf("network version mismatch: got %q", got)
	}

	if resolved.Sources["network"] != SourceFlag {
		t.Fatalf("network source mismatch: got %s", resolved.Sources["network"])
	}
	if resolved.Sources["validators"] != SourceFlag {
		t.Fatalf("validators source mismatch: got %s", resolved.Sources["validators"])
	}
	if resolved.Sources["mode"] != SourceEnvironment {
		t.Fatalf("mode source mismatch: got %s", resolved.Sources["mode"])
	}
	if resolved.Sources["network_version"] != SourceEnvironment {
		t.Fatalf("network version source mismatch: got %s", resolved.Sources["network_version"])
	}
}

func TestResolveRuntimeFileConfigPreservesNilDefaults(t *testing.T) {
	cmd := newRuntimeResolverTestCmd()

	resolver := NewResolver()
	resolved := resolver.ResolveRuntimeFileConfig(cmd, nil, RuntimeResolveInput{
		Network:           "mainnet",
		BlockchainNetwork: "stable",
		Validators:        4,
		Mode:              types.ExecutionModeDocker,
		NetworkVersion:    "latest",
		NoCache:           false,
		Accounts:          0,
	})

	if resolved.FileConfig.Network != nil {
		t.Fatalf("expected network to remain nil when only defaults are present")
	}
	if resolved.FileConfig.ExecutionMode != nil {
		t.Fatalf("expected mode to remain nil when only defaults are present")
	}
	if resolved.FileConfig.NetworkVersion != nil {
		t.Fatalf("expected network version to remain nil when only defaults are present")
	}
	if resolved.Sources["network"] != SourceDefault {
		t.Fatalf("expected default source for network, got %s", resolved.Sources["network"])
	}
}

func TestResolveGlobalPrecedence(t *testing.T) {
	t.Setenv("DEVNET_HOME", "/env-home")
	t.Setenv("NO_COLOR", "1")

	cmd := newGlobalResolverTestCmd()
	if err := cmd.Flags().Set("home", "/flag-home"); err != nil {
		t.Fatalf("set home flag: %v", err)
	}
	if err := cmd.Flags().Set("verbose", "true"); err != nil {
		t.Fatalf("set verbose flag: %v", err)
	}

	base := &FileConfig{
		Home:    strPtr("/file-home"),
		Verbose: boolPtrTest(false),
		JSON:    boolPtrTest(true),
		NoColor: boolPtrTest(false),
	}

	resolver := NewResolver()
	out := resolver.ResolveGlobal(cmd, base, GlobalResolveInput{
		Home:    "/flag-home",
		Verbose: true,
		JSON:    false,
		NoColor: false,
	})

	if out.Home.Value != "/flag-home" || out.Home.Source != SourceFlag {
		t.Fatalf("home resolution mismatch: value=%q source=%s", out.Home.Value, out.Home.Source)
	}
	if !out.Verbose.Value || out.Verbose.Source != SourceFlag {
		t.Fatalf("verbose resolution mismatch: value=%t source=%s", out.Verbose.Value, out.Verbose.Source)
	}
	if !out.JSON.Value || out.JSON.Source != SourceConfigFile {
		t.Fatalf("json resolution mismatch: value=%t source=%s", out.JSON.Value, out.JSON.Source)
	}
	if !out.NoColor.Value || out.NoColor.Source != SourceEnvironment {
		t.Fatalf("no-color resolution mismatch: value=%t source=%s", out.NoColor.Value, out.NoColor.Source)
	}
}

func TestResolveGlobalFlagBeatsEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	cmd := newGlobalResolverTestCmd()
	if err := cmd.Flags().Set("no-color", "false"); err != nil {
		t.Fatalf("set no-color flag: %v", err)
	}

	resolver := NewResolver()
	out := resolver.ResolveGlobal(cmd, &FileConfig{}, GlobalResolveInput{
		Home:    "/tmp/devnet",
		Verbose: false,
		JSON:    false,
		NoColor: false,
	})

	if out.NoColor.Source != SourceFlag {
		t.Fatalf("expected flag source, got %s", out.NoColor.Source)
	}
	if out.NoColor.Value {
		t.Fatalf("expected no-color=false from explicit flag")
	}
}

func newRuntimeResolverTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("network", "mainnet", "")
	cmd.Flags().String("blockchain", "stable", "")
	cmd.Flags().Int("validators", 4, "")
	cmd.Flags().String("mode", "docker", "")
	cmd.Flags().String("network-version", "latest", "")
	cmd.Flags().Bool("no-cache", false, "")
	cmd.Flags().Int("accounts", 0, "")
	return cmd
}

func newGlobalResolverTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("home", "/tmp/default-home", "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("no-color", false, "")
	return cmd
}

func strPtr(v string) *string { return &v }

func intPtrTest(v int) *int { return &v }

func boolPtrTest(v bool) *bool { return &v }

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func derefMode(v *types.ExecutionMode) types.ExecutionMode {
	if v == nil {
		return ""
	}
	return *v
}
