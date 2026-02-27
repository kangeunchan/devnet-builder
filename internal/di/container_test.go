package di

import (
	"context"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/output"
)

func TestNewContainerDefaults(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("expected container")
	}
	if c.Logger() == nil {
		t.Fatal("expected default logger")
	}
	if c.Config() == nil {
		t.Fatal("expected default config")
	}
	if c.NetworkRegistry() == nil {
		t.Fatal("expected network registry wrapper")
	}
}

func TestWithConfigInitializesPluginManager(t *testing.T) {
	c := New(WithConfig(&Config{PluginDir: t.TempDir()}))
	if c.PluginManager() == nil {
		t.Fatal("expected plugin manager to be initialized when PluginDir is set")
	}
}

func TestWireContainerBuildsCoreDependencies(t *testing.T) {
	factory := NewInfrastructureFactory(t.TempDir(), output.NewLogger()).WithNetworkModule(&testNetworkModule{})
	container, err := factory.WireContainer()
	if err != nil {
		t.Fatalf("WireContainer failed: %v", err)
	}

	if container.DevnetRepository() == nil {
		t.Fatal("expected devnet repository")
	}
	if container.NodeRepository() == nil {
		t.Fatal("expected node repository")
	}
	if container.ProvisionUseCase() == nil {
		t.Fatal("expected provision use case")
	}
	if container.ExecuteUpgradeUseCase() == nil {
		t.Fatal("expected execute upgrade use case")
	}
}

func TestContainerLazyAccessors(t *testing.T) {
	factory := NewInfrastructureFactory(t.TempDir(), output.NewLogger()).WithNetworkModule(&testNetworkModule{})
	container, err := factory.WireContainer()
	if err != nil {
		t.Fatalf("WireContainer failed: %v", err)
	}

	container.SetNetworkModule(nil)
	container.SetBinaryResolver(nil)

	_ = container.Logger()
	_ = container.LoggerPort()
	_ = container.NetworkRegistry()
	_ = container.PluginManager()
	_ = container.Config()
	_ = container.DevnetRepository()
	_ = container.NodeRepository()
	_ = container.ExportRepository()
	_ = container.Executor()
	_ = container.HealthChecker()
	_ = container.NetworkModule()
	_ = container.GitHubClient()
	_ = container.InteractiveSelector()
	_ = container.BinaryCache()
	_ = container.Builder()
	_ = container.ValidatorKeyLoader()
	_ = container.EVMClient()
	_ = container.RPCClient()
	_ = container.NodeInitializer()
	_ = container.ProvisionUseCase()
	_ = container.RunUseCase()
	_ = container.StopUseCase()
	_ = container.HealthUseCase()
	_ = container.ResetUseCase()
	_ = container.DestroyUseCase()
	_ = container.ProposeUseCase()
	_ = container.VoteUseCase()
	_ = container.SwitchBinaryUseCase()
	_ = container.ExecuteUpgradeUseCase()
	_ = container.MonitorUseCase()
	_ = container.StateManager()
	_ = container.StateTransitioner()
	_ = container.StateDetector()
	_ = container.ResumableExecuteUpgradeUseCase()
	_ = container.ResumeUseCase()
	_ = container.BuildUseCase()
	_ = container.CacheListUseCase()
	_ = container.CacheCleanUseCase()
	_ = container.PassthroughUseCase()
	_ = container.ImportCustomBinaryUseCase()
	_ = container.NodeLifecycleManager()
	_ = container.ExportUseCase(context.Background())
}
