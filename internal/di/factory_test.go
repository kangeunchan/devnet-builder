package di

import (
	"context"
	"testing"

	cosmoslog "cosmossdk.io/log"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/network"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/plugin"
	"github.com/altuslabsxyz/devnet-builder/internal/output"
	pkgnetwork "github.com/altuslabsxyz/devnet-builder/pkg/network"
)

type testNetworkModule struct{}

func (m *testNetworkModule) Name() string        { return "testnet" }
func (m *testNetworkModule) DisplayName() string { return "Testnet" }
func (m *testNetworkModule) Version() string     { return "1.0.0" }
func (m *testNetworkModule) BinaryName() string  { return "testd" }
func (m *testNetworkModule) BinarySource() network.BinarySource {
	return network.BinarySource{Type: network.BinarySourceGitHub, Owner: "owner", Repo: "repo"}
}
func (m *testNetworkModule) DefaultBinaryVersion() string { return "v1.0.0" }
func (m *testNetworkModule) GetBuildConfig(networkType string) (*pkgnetwork.BuildConfig, error) {
	return &pkgnetwork.BuildConfig{Tags: []string{"netgo"}}, nil
}
func (m *testNetworkModule) Bech32Prefix() string                 { return "test" }
func (m *testNetworkModule) BaseDenom() string                    { return "utest" }
func (m *testNetworkModule) GenesisConfig() network.GenesisConfig { return network.GenesisConfig{} }
func (m *testNetworkModule) DefaultChainID() string               { return "testnet-1" }
func (m *testNetworkModule) DockerImage() string                  { return "ghcr.io/example/testd" }
func (m *testNetworkModule) DockerImageTag(version string) string { return version }
func (m *testNetworkModule) DockerHomeDir() string                { return "/home/testd" }
func (m *testNetworkModule) InitCommand(homeDir, chainID, moniker string) []string {
	return []string{"init", moniker, "--chain-id", chainID}
}
func (m *testNetworkModule) StartCommand(homeDir string, networkMode string) []string {
	return []string{"start"}
}
func (m *testNetworkModule) ExportCommand(homeDir string) []string { return []string{"export"} }
func (m *testNetworkModule) DefaultMoniker(index int) string       { return "node0" }
func (m *testNetworkModule) DefaultNodeHome() string               { return ".testd" }
func (m *testNetworkModule) PIDFileName() string                   { return "testd.pid" }
func (m *testNetworkModule) LogFileName() string                   { return "testd.log" }
func (m *testNetworkModule) ProcessPattern() string                { return "testd" }
func (m *testNetworkModule) DefaultPorts() network.PortConfig      { return network.DefaultPortConfig() }
func (m *testNetworkModule) ConfigDir(homeDir string) string       { return homeDir + "/config" }
func (m *testNetworkModule) DataDir(homeDir string) string         { return homeDir + "/data" }
func (m *testNetworkModule) KeyringDir(homeDir string, backend string) string {
	return homeDir + "/keyring-" + backend
}
func (m *testNetworkModule) ModifyGenesis(genesis []byte, opts network.GenesisOptions) ([]byte, error) {
	return genesis, nil
}
func (m *testNetworkModule) NewGenerator(config *network.GeneratorConfig, logger cosmoslog.Logger) (network.Generator, error) {
	return nil, nil
}
func (m *testNetworkModule) DefaultGeneratorConfig() *network.GeneratorConfig {
	return &network.GeneratorConfig{}
}
func (m *testNetworkModule) Validate() error                       { return nil }
func (m *testNetworkModule) SnapshotURL(networkType string) string { return "https://snapshot.example" }
func (m *testNetworkModule) RPCEndpoint(networkType string) string { return "https://rpc.example" }
func (m *testNetworkModule) AvailableNetworks() []string           { return []string{"mainnet", "testnet"} }
func (m *testNetworkModule) GetConfigOverrides(nodeIndex int, opts network.NodeConfigOptions) ([]byte, []byte, error) {
	return []byte("config"), []byte("app"), nil
}

type fileCapableModule struct {
	*testNetworkModule
}

func (m *fileCapableModule) ModifyGenesisFile(inputPath, outputPath string, opts network.GenesisOptions) (int64, error) {
	return 42, nil
}

func TestOptionFunctionsApplyWithoutPanics(t *testing.T) {
	c := New()
	logger := output.NewLogger()
	pm := plugin.NewPluginManager(t.TempDir())

	opts := []Option{
		WithLogger(logger),
		WithConfig(&Config{HomeDir: t.TempDir(), PluginDir: t.TempDir()}),
		WithPluginManager(pm),
		WithDevnetRepository((ports.DevnetRepository)(nil)),
		WithNodeRepository((ports.NodeRepository)(nil)),
		WithBinaryCache((ports.BinaryCache)(nil)),
		WithExecutor((ports.ProcessExecutor)(nil)),
		WithRPCClient((ports.RPCClient)(nil)),
		WithEVMClient((ports.EVMClient)(nil)),
		WithSnapshotFetcher((ports.SnapshotFetcher)(nil)),
		WithGenesisFetcher((ports.GenesisFetcher)(nil)),
		WithStateExportService((ports.StateExportService)(nil)),
		WithNodeInitializer((ports.NodeInitializer)(nil)),
		WithKeyManager((ports.KeyManager)(nil)),
		WithHealthChecker((ports.HealthChecker)(nil)),
		WithValidatorKeyLoader((ports.ValidatorKeyLoader)(nil)),
		WithBuilder((ports.Builder)(nil)),
		WithNetworkModule((ports.NetworkModule)(nil)),
		WithGitHubClient((ports.GitHubClient)(nil)),
		WithInteractiveSelector((ports.InteractiveSelector)(nil)),
		WithBinaryResolver((ports.BinaryResolver)(nil)),
		WithBinaryExecutor((ports.BinaryExecutor)(nil)),
		WithExportRepository((ports.ExportRepository)(nil)),
		WithBinaryVersionDetector((ports.BinaryVersionDetector)(nil)),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.Logger() != logger {
		t.Fatal("expected logger option to be applied")
	}
	if c.PluginManager() != pm {
		t.Fatal("expected plugin manager option to be applied")
	}
}

func TestNetworkModuleAdapterMethods(t *testing.T) {
	base := &testNetworkModule{}
	adapter := &networkModuleAdapter{module: base}

	if adapter.Name() != "testnet" {
		t.Fatalf("unexpected adapter name: %s", adapter.Name())
	}
	if adapter.BinaryName() != "testd" {
		t.Fatalf("unexpected binary name: %s", adapter.BinaryName())
	}
	if adapter.DockerImageTag("v1") != "v1" {
		t.Fatal("expected docker image tag passthrough")
	}
	if _, err := adapter.ModifyGenesis([]byte(`{}`), ports.GenesisModifyOptions{}); err != nil {
		t.Fatalf("unexpected ModifyGenesis error: %v", err)
	}

	portsCfg := adapter.DefaultPorts()
	if portsCfg.PProf != 6060 || portsCfg.Rosetta != 8080 {
		t.Fatalf("unexpected default extra ports: %+v", portsCfg)
	}

	if _, err := adapter.ModifyGenesisFile("in", "out", ports.GenesisModifyOptions{}); err == nil {
		t.Fatal("expected ModifyGenesisFile to fail when module does not support file modifier")
	}

	fileAdapter := &networkModuleAdapter{module: &fileCapableModule{testNetworkModule: base}}
	size, err := fileAdapter.ModifyGenesisFile("in", "out", ports.GenesisModifyOptions{})
	if err != nil {
		t.Fatalf("unexpected file ModifyGenesisFile error: %v", err)
	}
	if size != 42 {
		t.Fatalf("unexpected output size: %d", size)
	}
}

func TestInfrastructureFactoryCreateMethods(t *testing.T) {
	logger := output.NewLogger()
	module := &testNetworkModule{}
	f := NewInfrastructureFactory(t.TempDir(), logger).
		WithNetworkModule(module).
		WithGitHubConfig("token", "owner", "repo")

	if f.CreateDevnetRepository() == nil {
		t.Fatal("expected devnet repository")
	}
	if f.CreateNodeRepository() == nil {
		t.Fatal("expected node repository")
	}
	if f.CreateExportRepository() == nil {
		t.Fatal("expected export repository")
	}
	if f.CreateDockerExecutor() == nil {
		t.Fatal("expected docker executor")
	}
	if f.CreateRPCClient("localhost", 26657) == nil {
		t.Fatal("expected rpc client")
	}
	if _, err := f.CreateBinaryCache(); err != nil {
		t.Fatalf("expected binary cache, got error: %v", err)
	}
	if f.CreateBinaryVersionDetector() == nil {
		t.Fatal("expected version detector")
	}
	if f.CreateBuilder() == nil {
		t.Fatal("expected builder")
	}
	if f.CreateSnapshotFetcher() == nil {
		t.Fatal("expected snapshot fetcher")
	}
	if f.CreateGenesisFetcher() == nil {
		t.Fatal("expected genesis fetcher")
	}
	if f.CreateStateExportService() == nil {
		t.Fatal("expected state export service")
	}
	if f.CreateNodeInitializer() == nil {
		t.Fatal("expected node initializer")
	}
	if f.CreateNodeManagerFactory() == nil {
		t.Fatal("expected node manager factory")
	}
	if f.CreateGitHubClient() == nil {
		t.Fatal("expected github client")
	}
	if f.CreateInteractiveSelector() == nil {
		t.Fatal("expected interactive selector")
	}
	if f.CreateEVMClient("http://localhost:8545") == nil {
		t.Fatal("expected evm client")
	}
	if f.CreateValidatorKeyLoader() == nil {
		t.Fatal("expected validator key loader")
	}
	if f.CreateVersionRepository() == nil {
		t.Fatal("expected version repository")
	}
	if f.CreateMigrationService() == nil {
		t.Fatal("expected migration service")
	}
	if f.CreateBinaryExecutor() == nil {
		t.Fatal("expected binary executor")
	}
	if f.CreateBinaryResolver(nil, nil) == nil {
		t.Fatal("expected binary resolver")
	}

	f.WithDockerMode(false)
	if f.CreateProcessExecutor() == nil {
		t.Fatal("expected local process executor")
	}
	f.WithDockerMode(true)
	if f.CreateProcessExecutor() == nil {
		t.Fatal("expected docker process executor")
	}

	health := f.CreateHealthChecker(26657)
	results, err := health.CheckAllNodes(context.Background(), []*ports.NodeMetadata{})
	if err != nil {
		t.Fatalf("unexpected CheckAllNodes error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 health results, got %d", len(results))
	}

	container, err := f.WireContainer()
	if err != nil {
		t.Fatalf("WireContainer failed: %v", err)
	}
	if container == nil {
		t.Fatal("expected wired container")
	}
}
