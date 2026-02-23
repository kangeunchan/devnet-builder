package unit

import (
	"reflect"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func TestModuleSurface_DefaultMetadataAndCommands(t *testing.T) {
	networkModule := cosmos.New()

	if got := networkModule.Name(); got != "cosmos" {
		t.Fatalf("Name() = %q", got)
	}
	if got := networkModule.DisplayName(); got != "Cosmos Hub" {
		t.Fatalf("DisplayName() = %q", got)
	}
	if got := networkModule.Version(); got == "" {
		t.Fatal("Version() should not be empty")
	}

	binarySource := networkModule.BinarySource()
	if binarySource.Owner != "cosmos" || binarySource.Repo != "gaia" {
		t.Fatalf("unexpected binary source: %+v", binarySource)
	}
	if got := networkModule.DefaultBinaryVersion(); got == "" {
		t.Fatal("DefaultBinaryVersion() should not be empty")
	}

	initCmd := networkModule.InitCommand("/tmp/node0", "cosmosdevnet-1", "validator0")
	if !containsArgPair(initCmd, "--chain-id", "cosmosdevnet-1") {
		t.Fatalf("unexpected init command: %v", initCmd)
	}
	if !containsArgPair(initCmd, "--home", "/tmp/node0") {
		t.Fatalf("unexpected init command: %v", initCmd)
	}

	exportCmd := networkModule.ExportCommand("/tmp/node0")
	if !containsArgPair(exportCmd, "--home", "/tmp/node0") {
		t.Fatalf("unexpected export command: %v", exportCmd)
	}

	if got := networkModule.RPCEndpoints("unknown"); got != nil {
		t.Fatalf("RPCEndpoints(unknown) = %v, want nil", got)
	}
	if got := networkModule.SnapshotURLs("unknown"); got != nil {
		t.Fatalf("SnapshotURLs(unknown) = %v, want nil", got)
	}
}

func TestModuleSurface_RuntimeOverridesAndNetworkOrdering(t *testing.T) {
	networkModule := cosmos.New(cosmos.WithCustomization(cosmos.Customization{
		Runtime: cosmos.RuntimeCustomization{
			BinaryName:    "customd",
			DockerImage:   "example/custom:v1",
			DockerHomeDir: "/custom/home",
			DefaultHome:   "/custom/.home",
		},
		NetworkProfiles: map[string]cosmos.NetworkProfileConfig{
			"beta": {
				ChainID:          "beta-1",
				RPCEndpoint:      "https://beta-rpc.example.com",
				RESTEndpoint:     "https://beta-rest.example.com",
				SnapshotIndexURL: "https://beta-snapshots.example.com",
			},
			"alpha": {
				ChainID:          "alpha-1",
				RPCEndpoint:      "https://alpha-rpc.example.com",
				RESTEndpoint:     "https://alpha-rest.example.com",
				SnapshotIndexURL: "https://alpha-snapshots.example.com",
			},
		},
	}))

	if got := networkModule.BinaryName(); got != "customd" {
		t.Fatalf("BinaryName() = %q", got)
	}
	if got := networkModule.DockerImage(); got != "example/custom:v1" {
		t.Fatalf("DockerImage() = %q", got)
	}
	if got := networkModule.DockerHomeDir(); got != "/custom/home" {
		t.Fatalf("DockerHomeDir() = %q", got)
	}
	if got := networkModule.DefaultNodeHome(); got != "/custom/.home" {
		t.Fatalf("DefaultNodeHome() = %q", got)
	}

	wantNetworks := []string{"mainnet", "testnet", "alpha", "beta"}
	if got := networkModule.AvailableNetworks(); !reflect.DeepEqual(got, wantNetworks) {
		t.Fatalf("AvailableNetworks() = %v, want %v", got, wantNetworks)
	}

	wantRPC := []string{"https://cosmoshub.rpc.kjnodes.com"}
	if got := networkModule.RPCEndpoints("mainnet"); !reflect.DeepEqual(got, wantRPC) {
		t.Fatalf("RPCEndpoints(mainnet) = %v, want %v", got, wantRPC)
	}
}
