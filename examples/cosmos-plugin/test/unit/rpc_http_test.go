package unit

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on tcp4 test port: %v", err)
	}

	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	return "http://" + listener.Addr().String()
}

func TestGetBlockHeight_FromStatusEndpoint(t *testing.T) {
	networkModule := cosmos.New()

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"result":{"sync_info":{"latest_block_height":"12345"}}}`))
	})

	resp, err := networkModule.GetBlockHeight(context.Background(), srv)
	if err != nil {
		t.Fatalf("GetBlockHeight returned error: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("GetBlockHeight returned response error: %s", resp.Error)
	}
	if resp.Height != 12345 {
		t.Fatalf("unexpected height: %d", resp.Height)
	}
}

func TestGetGovernanceParams_FromRESTEndpoints(t *testing.T) {
	networkModule := cosmos.New()

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cosmos/gov/v1/params/voting":
			_, _ = w.Write([]byte(`{"voting_params":{"voting_period":"60s","expedited_voting_period":"30s"}}`))
		case "/cosmos/gov/v1/params/deposit":
			_, _ = w.Write([]byte(`{"deposit_params":{"min_deposit":[{"denom":"uatom","amount":"10000000"}],"expedited_min_deposit":[{"denom":"uatom","amount":"50000000"}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	resp, err := networkModule.GetGovernanceParams(srv, "")
	if err != nil {
		t.Fatalf("GetGovernanceParams returned error: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("GetGovernanceParams returned response error: %s", resp.Error)
	}
	if resp.VotingPeriodNs != int64(60*time.Second) {
		t.Fatalf("unexpected voting period ns: %d", resp.VotingPeriodNs)
	}
	if resp.MinDeposit != "10000000" {
		t.Fatalf("unexpected min deposit: %s", resp.MinDeposit)
	}
	if resp.ExpeditedMinDeposit != "50000000" {
		t.Fatalf("unexpected expedited min deposit: %s", resp.ExpeditedMinDeposit)
	}
}

func TestGetProposal_FromRESTEndpoint(t *testing.T) {
	networkModule := cosmos.New()

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cosmos/gov/v1/proposals/7" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"proposal":{"id":"7","title":"Upgrade","summary":"desc","metadata":"","status":"PROPOSAL_STATUS_VOTING_PERIOD","submit_time":"2025-01-01T00:00:00Z","deposit_end_time":"2025-01-01T00:10:00Z","voting_end_time":"2025-01-01T00:20:00Z","total_deposit":[{"denom":"uatom","amount":"123"}],"final_tally_result":{"yes_count":"1","no_count":"2","abstain_count":"3"}}}`))
	})

	resp, err := networkModule.GetProposal(context.Background(), srv, 7)
	if err != nil {
		t.Fatalf("GetProposal returned error: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("GetProposal returned response error: %s", resp.Error)
	}
	if resp.Id != 7 {
		t.Fatalf("unexpected proposal id: %d", resp.Id)
	}
	if resp.TotalDeposit != "123" {
		t.Fatalf("unexpected total deposit: %s", resp.TotalDeposit)
	}
}

func TestGetUpgradePlan_NoPlan(t *testing.T) {
	networkModule := cosmos.New()

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cosmos/upgrade/v1beta1/current_plan" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"plan":null}`))
	})

	resp, err := networkModule.GetUpgradePlan(context.Background(), srv)
	if err != nil {
		t.Fatalf("GetUpgradePlan returned error: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("GetUpgradePlan returned response error: %s", resp.Error)
	}
	if resp.HasPlan {
		t.Fatalf("expected no upgrade plan")
	}
}

func TestWaitForBlock_Timeout(t *testing.T) {
	networkModule := cosmos.New()

	resp, err := networkModule.WaitForBlock(context.Background(), "http://127.0.0.1:1", 10, 50)
	if err != nil {
		t.Fatalf("WaitForBlock returned error: %v", err)
	}
	if resp.Reached {
		t.Fatalf("expected timeout result, got reached")
	}
	if !strings.Contains(resp.Error, "timeout") {
		t.Fatalf("expected timeout message, got %q", resp.Error)
	}
}

func TestGetBlockHeight_InvalidHeightResponse(t *testing.T) {
	networkModule := cosmos.New()

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"result":{"sync_info":{"latest_block_height":"not-a-number"}}}`))
	})

	resp, err := networkModule.GetBlockHeight(context.Background(), srv)
	if err != nil {
		t.Fatalf("GetBlockHeight returned error: %v", err)
	}
	if !strings.Contains(resp.Error, "failed to parse latest_block_height") {
		t.Fatalf("unexpected response error: %q", resp.Error)
	}
}

func TestGetGovernanceParams_InvalidVotingPeriod(t *testing.T) {
	networkModule := cosmos.New()

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cosmos/gov/v1/params/voting":
			_, _ = w.Write([]byte(`{"voting_params":{"voting_period":"bad-duration"}}`))
		case "/cosmos/gov/v1/params/deposit":
			_, _ = w.Write([]byte(`{"deposit_params":{"min_deposit":[{"denom":"uatom","amount":"10000000"}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	resp, err := networkModule.GetGovernanceParams(srv, "")
	if err != nil {
		t.Fatalf("GetGovernanceParams returned error: %v", err)
	}
	if !strings.Contains(resp.Error, "invalid voting_period") {
		t.Fatalf("unexpected response error: %q", resp.Error)
	}
}

func TestGetProposal_InvalidJSONResponse(t *testing.T) {
	networkModule := cosmos.New()

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cosmos/gov/v1/proposals/9" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"proposal":`))
	})

	resp, err := networkModule.GetProposal(context.Background(), srv, 9)
	if err != nil {
		t.Fatalf("GetProposal returned error: %v", err)
	}
	if resp.Error == "" {
		t.Fatalf("expected parse error response, got empty error")
	}
}
