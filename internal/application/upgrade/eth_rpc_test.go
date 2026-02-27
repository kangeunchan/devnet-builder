package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func startFakeEthRPCServer(t *testing.T, proposalID uint64) func() {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:8545")
	if err != nil {
		t.Skipf("cannot bind fake eth rpc on :8545: %v", err)
	}

	txHash := "0x" + strings.Repeat("1", 64)
	blockHash := "0x" + strings.Repeat("2", 64)
	logsBloom := "0x" + strings.Repeat("0", 512)
	proposalIDTopic := fmt.Sprintf("0x%064x", proposalID)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: nil, Error: map[string]interface{}{"code": -32700, "message": "parse error"}})
			return
		}

		res := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "eth_chainId":
			res.Result = "0x1"
		case "eth_getBalance":
			res.Result = "0x56bc75e2d63100000"
		case "eth_getTransactionCount":
			res.Result = "0x0"
		case "eth_gasPrice":
			res.Result = "0x3b9aca00"
		case "eth_call":
			res.Result = "0x"
		case "eth_estimateGas":
			res.Result = "0x7a120"
		case "eth_sendRawTransaction":
			res.Result = txHash
		case "eth_getTransactionReceipt":
			res.Result = map[string]interface{}{
				"transactionHash":   txHash,
				"transactionIndex":  "0x0",
				"blockHash":         blockHash,
				"blockNumber":       "0x1",
				"from":              "0x0000000000000000000000000000000000000001",
				"to":                GovPrecompileAddress,
				"cumulativeGasUsed": "0x5208",
				"gasUsed":           "0x5208",
				"contractAddress":   nil,
				"logsBloom":         logsBloom,
				"status":            "0x1",
				"type":              "0x0",
				"effectiveGasPrice": "0x3b9aca00",
				"logs": []map[string]interface{}{
					{
						"address":          GovPrecompileAddress,
						"topics":           []string{"0x" + strings.Repeat("3", 64), proposalIDTopic},
						"data":             "0x",
						"blockNumber":      "0x1",
						"transactionHash":  txHash,
						"transactionIndex": "0x0",
						"blockHash":        blockHash,
						"logIndex":         "0x0",
						"removed":          false,
					},
				},
			}
		default:
			res.Error = map[string]interface{}{"code": -32601, "message": "method not found"}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	srv := &http.Server{Handler: handler}
	go func() {
		_ = srv.Serve(listener)
	}()

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

func testValidatorKey(t *testing.T) ports.ValidatorKey {
	t.Helper()
	pkHex := "4f3edf983ac636a65a842ce7c78d9aa706d3b113bce036f0f6078ff8b8f4f0c7"
	pk, err := crypto.HexToECDSA(pkHex)
	if err != nil {
		t.Fatalf("failed to parse key: %v", err)
	}
	addr := crypto.PubkeyToAddress(pk.PublicKey).Hex()
	return ports.ValidatorKey{
		Name:          "validator0",
		Bech32Address: "stable1test",
		HexAddress:    addr,
		PrivateKey:    pkHex,
	}
}

func TestProposeUseCase_Execute_FakeEthRPC(t *testing.T) {
	teardown := startFakeEthRPCServer(t, 7)
	defer teardown()

	key := testValidatorKey(t)
	uc := NewProposeUseCase(
		&mockDevnetRepo{},
		&mockRPCClient{},
		&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
			return []ports.ValidatorKey{key}, nil
		}},
		&testLogger{},
	)

	out, err := uc.Execute(context.Background(), dto.ProposeInput{
		HomeDir:       "/tmp/devnet",
		UpgradeName:   "v2.0.0",
		UpgradeHeight: 12345,
		VotingPeriod:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ProposalID != 7 {
		t.Fatalf("proposal id = %d, want 7", out.ProposalID)
	}
	if out.TxHash == "" {
		t.Fatalf("expected tx hash")
	}
}

func TestVoteUseCase_Execute_FakeEthRPC(t *testing.T) {
	teardown := startFakeEthRPCServer(t, 9)
	defer teardown()

	key := testValidatorKey(t)
	uc := NewVoteUseCase(
		&mockDevnetRepo{loadFunc: func(ctx context.Context, homeDir string) (*ports.DevnetMetadata, error) {
			return &ports.DevnetMetadata{NumValidators: 1, ExecutionMode: "local", CurrentVersion: "v1", BinaryName: "stabled"}, nil
		}},
		&mockRPCClient{getProposalFunc: func(ctx context.Context, id uint64) (*ports.Proposal, error) {
			return &ports.Proposal{Status: ports.ProposalStatusVoting}, nil
		}},
		&mockValidatorKeyLoader{loadValidatorKeysFunc: func(ctx context.Context, opts ports.ValidatorKeyOptions) ([]ports.ValidatorKey, error) {
			return []ports.ValidatorKey{key}, nil
		}},
		&testLogger{},
	)

	out, err := uc.Execute(context.Background(), dto.VoteInput{
		HomeDir:    "/tmp/devnet",
		ProposalID: 9,
		VoteOption: "yes",
		FromAll:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.VotesCast != 1 || out.TotalVoters != 1 {
		t.Fatalf("unexpected vote result: %+v", out)
	}
	if len(out.TxHashes) != 1 {
		t.Fatalf("expected one tx hash")
	}
}

func TestWaitForReceiptCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := waitForReceipt(ctx, nil, common.Hash{})
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
}

func TestGetLatestProposalID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"proposals":[{"id":"42"}]}`))
	}))
	defer ts.Close()

	id, err := getLatestProposalID(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d", id)
	}
}
