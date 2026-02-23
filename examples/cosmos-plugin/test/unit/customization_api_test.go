package unit

import (
	"strings"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func TestCustomizationYAML_RoundTrip_Defaults(t *testing.T) {
	defaultCfg := cosmos.DefaultCustomization()

	yamlBytes, err := cosmos.EncodeCustomizationYAML(defaultCfg)
	if err != nil {
		t.Fatalf("EncodeCustomizationYAML returned error: %v", err)
	}

	decoded, err := cosmos.DecodeCustomizationYAML(yamlBytes)
	if err != nil {
		t.Fatalf("DecodeCustomizationYAML returned error: %v", err)
	}

	if decoded.NetworkProfiles["mainnet"].RPCEndpoint != defaultCfg.NetworkProfiles["mainnet"].RPCEndpoint {
		t.Fatalf("mainnet rpc endpoint mismatch: want=%q got=%q", defaultCfg.NetworkProfiles["mainnet"].RPCEndpoint, decoded.NetworkProfiles["mainnet"].RPCEndpoint)
	}
	if decoded.NetworkProfiles["testnet"].RPCEndpoint != defaultCfg.NetworkProfiles["testnet"].RPCEndpoint {
		t.Fatalf("testnet rpc endpoint mismatch: want=%q got=%q", defaultCfg.NetworkProfiles["testnet"].RPCEndpoint, decoded.NetworkProfiles["testnet"].RPCEndpoint)
	}
	if decoded.Snapshot.MainnetURLPattern != defaultCfg.Snapshot.MainnetURLPattern {
		t.Fatalf("mainnet snapshot pattern mismatch: want=%q got=%q", defaultCfg.Snapshot.MainnetURLPattern, decoded.Snapshot.MainnetURLPattern)
	}
	if decoded.Runtime.DockerImage != defaultCfg.Runtime.DockerImage {
		t.Fatalf("docker image mismatch: want=%q got=%q", defaultCfg.Runtime.DockerImage, decoded.Runtime.DockerImage)
	}
	if decoded.GenesisPolicy.RequireValidators == nil || !*decoded.GenesisPolicy.RequireValidators {
		t.Fatalf("expected require_validators=true after round-trip, got %#v", decoded.GenesisPolicy.RequireValidators)
	}
	if len(decoded.Endpoints.RESTToRPCHostMap) != 0 || len(decoded.Endpoints.RPCToRESTHostMap) != 0 {
		t.Fatalf("expected empty endpoint override maps, got %+v", decoded.Endpoints)
	}
}

func TestDecodeCustomizationYAML_InvalidDurationFails(t *testing.T) {
	_, err := cosmos.DecodeCustomizationYAML([]byte(`
timeouts:
  request_timeout: definitely-not-duration
`))
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
	if !strings.Contains(err.Error(), "timeouts.request_timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEncodeCustomizationYAML_InvalidRPCPolicyFails(t *testing.T) {
	cfg := cosmos.DefaultCustomization()
	cfg.RPCPolicy.UnsupportedNetworkBehavior = "unsupported-policy"

	_, err := cosmos.EncodeCustomizationYAML(cfg)
	if err == nil {
		t.Fatal("expected unsupported rpc policy error")
	}
	if !strings.Contains(err.Error(), "rpc_policy.unsupported_network_behavior") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithCustomizationFile_EmptyPathSetsInitError(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithCustomizationFile("   "))

	err := networkModule.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty customization file path")
	}
	if !strings.Contains(err.Error(), "customization file path is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
