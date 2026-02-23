package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func TestRootCmdValidate(t *testing.T) {
	cmd := newRootCmd(cosmos.New())

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := out.String(); got != "ok\n" {
		t.Fatalf("expected output %q, got %q", "ok\n", got)
	}
}

func TestRootCmdValidateRejectsArgs(t *testing.T) {
	cmd := newRootCmd(cosmos.New())

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate", "unexpected"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unexpected argument")
	}

	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootCmdPrintTemplate(t *testing.T) {
	cmd := newRootCmd(cosmos.New())

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"print-template"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "network_profiles:") {
		t.Fatalf("expected template output, got %q", rendered)
	}
	if _, err := cosmos.DecodeCustomizationYAML([]byte(rendered)); err != nil {
		t.Fatalf("print-template should output valid customization yaml: %v", err)
	}
}

func TestNewNetworkModuleFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "customization.yaml")

	yamlBytes, err := cosmos.EncodeCustomizationYAML(cosmos.DefaultCustomization())
	if err != nil {
		t.Fatalf("failed to render default customization: %v", err)
	}
	if err := os.WriteFile(configPath, yamlBytes, 0o644); err != nil {
		t.Fatalf("failed to write customization file: %v", err)
	}

	t.Setenv("COSMOS_PLUGIN_CONFIG", configPath)
	module := newNetworkModuleFromEnv()
	if err := module.Validate(); err != nil {
		t.Fatalf("expected env-loaded module to validate: %v", err)
	}
}

func TestNewNetworkModuleFromEnv_InvalidFilePath(t *testing.T) {
	t.Setenv("COSMOS_PLUGIN_CONFIG", "/path/that/does/not/exist.yaml")
	module := newNetworkModuleFromEnv()

	err := module.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid COSMOS_PLUGIN_CONFIG path")
	}
	if !strings.Contains(err.Error(), "load customization file") {
		t.Fatalf("unexpected error: %v", err)
	}
}
