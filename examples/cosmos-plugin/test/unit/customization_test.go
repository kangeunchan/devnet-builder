package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func TestLoadCustomizationFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "customization.yaml")

	raw := `
network_profiles:
  mainnet:
    chain_id: cosmoshub-4
    rpc_endpoint: https://example-rpc.local
    rest_endpoint: https://example-rest.local
    snapshot_index_url: https://example-snapshot.local
timeouts:
  request_timeout: 20s
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	cfg, err := cosmos.LoadCustomizationFile(path)
	if err != nil {
		t.Fatalf("LoadCustomizationFile returned error: %v", err)
	}

	if cfg.NetworkProfiles["mainnet"].RPCEndpoint != "https://example-rpc.local" {
		t.Fatalf("unexpected mainnet rpc endpoint: %q", cfg.NetworkProfiles["mainnet"].RPCEndpoint)
	}
	if cfg.Timeouts.RequestTimeout != "20s" {
		t.Fatalf("unexpected request timeout: %q", cfg.Timeouts.RequestTimeout)
	}
}

func TestLoadCustomizationFile_UnknownFieldFails(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "customization.yaml")

	raw := `
network_profiles:
  mainnet:
    chain_id: cosmoshub-4
unknown_field: true
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := cosmos.LoadCustomizationFile(path)
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCustomizationPrecedence_FileThenOption(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "customization.yaml")

	raw := `
network_profiles:
  mainnet:
    chain_id: cosmoshub-4
    rpc_endpoint: https://file-rpc.local
    rest_endpoint: https://file-rest.local
    snapshot_index_url: https://file-snapshot.local
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	networkModule := cosmos.New(
		cosmos.WithCustomizationFile(path),
		cosmos.WithCustomization(cosmos.Customization{
			NetworkProfiles: map[string]cosmos.NetworkProfileConfig{
				"mainnet": {
					RPCEndpoint: "https://option-rpc.local",
				},
			},
		}),
	)

	if err := networkModule.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if got := networkModule.RPCEndpoint("mainnet"); got != "https://option-rpc.local" {
		t.Fatalf("unexpected rpc endpoint by precedence: %q", got)
	}
}

func TestLoadCustomizationFile_InvalidSnapshotPatternFails(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "customization.yaml")

	raw := `
snapshot:
  mainnet_url_pattern: "[invalid"
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := cosmos.LoadCustomizationFile(path)
	if err == nil {
		t.Fatalf("expected error for invalid snapshot pattern")
	}
	if !strings.Contains(err.Error(), "snapshot.mainnet_url_pattern") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCustomization_InvalidSnapshotPatternFailsValidation(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithCustomization(cosmos.Customization{
		Snapshot: cosmos.SnapshotCustomization{
			MainnetURLPattern: "[invalid",
		},
	}))

	err := networkModule.Validate()
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "snapshot.mainnet_url_pattern") {
		t.Fatalf("unexpected error: %v", err)
	}
}
