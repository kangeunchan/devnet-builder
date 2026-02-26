package devnet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/types"
)

type provisionTestLogger struct{}

func (provisionTestLogger) Info(string, ...interface{})    {}
func (provisionTestLogger) Warn(string, ...interface{})    {}
func (provisionTestLogger) Error(string, ...interface{})   {}
func (provisionTestLogger) Debug(string, ...interface{})   {}
func (provisionTestLogger) Success(string, ...interface{}) {}
func (provisionTestLogger) Print(string, ...interface{})   {}
func (provisionTestLogger) Println(string, ...interface{}) {}
func (provisionTestLogger) SetVerbose(bool)                {}
func (provisionTestLogger) IsVerbose() bool                { return false }
func (provisionTestLogger) Writer() io.Writer              { return io.Discard }
func (provisionTestLogger) ErrWriter() io.Writer           { return io.Discard }

type provisionTestDevnetRepo struct {
	ports.DevnetRepository
	exists  bool
	saveErr error
	saved   []*ports.DevnetMetadata
}

func (m *provisionTestDevnetRepo) Exists(string) bool { return m.exists }
func (m *provisionTestDevnetRepo) Save(_ context.Context, metadata *ports.DevnetMetadata) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, metadata)
	return nil
}

type provisionTestNodeRepo struct {
	ports.NodeRepository
	saveErr error
	saved   []*ports.NodeMetadata
}

func (m *provisionTestNodeRepo) Save(_ context.Context, node *ports.NodeMetadata) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, node)
	return nil
}

type provisionTestGenesisFetcher struct {
	ports.GenesisFetcher
	genesis []byte
	err     error
}

func (m provisionTestGenesisFetcher) FetchFromRPC(context.Context, string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.genesis, nil
}

type provisionTestNetworkModule struct {
	ports.NetworkModule
	rpcEndpoint string
	bech32      string
}

func (m provisionTestNetworkModule) RPCEndpoint(string) string { return m.rpcEndpoint }
func (m provisionTestNetworkModule) Bech32Prefix() string      { return m.bech32 }

type provisionTestNodeInitializer struct {
	ports.NodeInitializer
	createAccountKeyErr error
}

func (m provisionTestNodeInitializer) CreateAccountKey(context.Context, string, string) (*ports.AccountKeyInfo, error) {
	if m.createAccountKeyErr != nil {
		return nil, m.createAccountKeyErr
	}
	return &ports.AccountKeyInfo{}, nil
}

func TestPrepareMetadata(t *testing.T) {
	t.Run("returns error when devnet already exists", func(t *testing.T) {
		uc := &ProvisionUseCase{
			devnetRepo: &provisionTestDevnetRepo{exists: true},
			logger:     provisionTestLogger{},
		}

		_, err := uc.prepareMetadata(dto.ProvisionInput{HomeDir: "/tmp/existing"})
		if err == nil || !strings.Contains(err.Error(), "devnet already exists") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("resolves docker mode and RPC endpoint", func(t *testing.T) {
		uc := &ProvisionUseCase{
			devnetRepo:    &provisionTestDevnetRepo{},
			networkModule: provisionTestNetworkModule{rpcEndpoint: "https://rpc.example", bech32: "stable"},
			logger:        provisionTestLogger{},
		}

		state, err := uc.prepareMetadata(dto.ProvisionInput{
			HomeDir:       "/tmp/devnet",
			Mode:          string(types.ExecutionModeDocker),
			Network:       "mainnet",
			StableVersion: "v1.0.0",
		})
		if err != nil {
			t.Fatalf("prepareMetadata failed: %v", err)
		}
		if state.metadata.ExecutionMode != types.ExecutionModeDocker {
			t.Fatalf("execution mode mismatch: %s", state.metadata.ExecutionMode)
		}
		if state.rpcEndpoint != "https://rpc.example" {
			t.Fatalf("rpc endpoint mismatch: %s", state.rpcEndpoint)
		}
	})
}

func TestFetchGenesis(t *testing.T) {
	uc := &ProvisionUseCase{
		genesisSvc: provisionTestGenesisFetcher{
			genesis: []byte(`{"chain_id":"stable-devnet-1"}`),
		},
		logger: provisionTestLogger{},
	}

	state := &provisionPipelineState{
		metadata:    &ports.DevnetMetadata{},
		rpcEndpoint: "https://rpc.example",
	}

	if err := uc.fetchGenesis(context.Background(), dto.ProvisionInput{}, state); err != nil {
		t.Fatalf("fetchGenesis failed: %v", err)
	}
	if state.chainID != "stable-devnet-1" {
		t.Fatalf("chain id mismatch: %s", state.chainID)
	}
	if state.metadata.ChainID != state.chainID {
		t.Fatalf("metadata chain id mismatch: %s", state.metadata.ChainID)
	}
	if !bytes.Equal(state.genesis, []byte(`{"chain_id":"stable-devnet-1"}`)) {
		t.Fatal("expected genesis to match fetched rpc genesis when snapshot is disabled")
	}
}

func TestInitializeKeysAndNodes_FailsOnCreateAccountKeys(t *testing.T) {
	uc := &ProvisionUseCase{
		nodeInitializer: provisionTestNodeInitializer{
			createAccountKeyErr: errors.New("keyring unavailable"),
		},
		logger: provisionTestLogger{},
	}

	state := &provisionPipelineState{
		accountsDir: t.TempDir(),
		chainID:     "stable-devnet-1",
	}

	err := uc.initializeKeysAndNodes(context.Background(), dto.ProvisionInput{
		HomeDir:         t.TempDir(),
		NumValidators:   1,
		UseTestMnemonic: false,
	}, state)
	if err == nil || !strings.Contains(err.Error(), "failed to create account keys") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchGenesis(t *testing.T) {
	uc := &ProvisionUseCase{logger: provisionTestLogger{}}
	nodeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(nodeHome, "config"), 0755); err != nil {
		t.Fatalf("failed to create node config dir: %v", err)
	}

	state := &provisionPipelineState{
		metadata: &ports.DevnetMetadata{},
		genesis:  []byte(`{"chain_id":"stable-devnet-1"}`),
		chainID:  "stable-devnet-1",
		nodes: []*ports.NodeMetadata{
			{Index: 0, HomeDir: nodeHome},
		},
	}

	if err := uc.patchGenesis(context.Background(), dto.ProvisionInput{
		HomeDir:       nodeHome,
		NumValidators: 1,
	}, state); err != nil {
		t.Fatalf("patchGenesis failed: %v", err)
	}

	if state.metadata.Status != ports.StateProvisioned {
		t.Fatalf("unexpected metadata status: %s", state.metadata.Status)
	}
	if state.metadata.LastProvisioned == nil {
		t.Fatal("expected LastProvisioned to be set")
	}
	if state.metadata.GenesisPath == "" {
		t.Fatal("expected genesis path to be set")
	}

	written, err := os.ReadFile(state.metadata.GenesisPath)
	if err != nil {
		t.Fatalf("failed to read written genesis: %v", err)
	}
	if !bytes.Equal(written, state.genesis) {
		t.Fatalf("written genesis mismatch: %s", string(written))
	}
}

func TestPersistState(t *testing.T) {
	devnetRepo := &provisionTestDevnetRepo{}
	nodeRepo := &provisionTestNodeRepo{}
	uc := &ProvisionUseCase{
		devnetRepo: devnetRepo,
		nodeRepo:   nodeRepo,
		logger:     provisionTestLogger{},
	}

	state := &provisionPipelineState{
		metadata: &ports.DevnetMetadata{
			ChainID:     "stable-devnet-1",
			GenesisPath: "/tmp/genesis.json",
		},
		nodes: []*ports.NodeMetadata{
			{Index: 0, Name: "node0", HomeDir: "/tmp/node0", NodeID: "id-0"},
			{Index: 1, Name: "node1", HomeDir: "/tmp/node1", NodeID: "id-1"},
		},
	}

	output, err := uc.persistState(context.Background(), dto.ProvisionInput{
		HomeDir:       "/tmp/devnet",
		NumValidators: 2,
		NumAccounts:   1,
	}, state)
	if err != nil {
		t.Fatalf("persistState failed: %v", err)
	}

	if len(devnetRepo.saved) != 1 {
		t.Fatalf("expected one metadata save, got %d", len(devnetRepo.saved))
	}
	if len(nodeRepo.saved) != 2 {
		t.Fatalf("expected two node saves, got %d", len(nodeRepo.saved))
	}
	if output.ChainID != "stable-devnet-1" || len(output.Nodes) != 2 {
		t.Fatalf("unexpected output: %+v", output)
	}
}
