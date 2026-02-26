package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
)

func TestGetBlockHeight_FromStatusEndpoint(t *testing.T) {
	networkModule := cosmos.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"result":{"sync_info":{"latest_block_height":"12345"}}}`))
	}))
	defer srv.Close()

	resp, err := networkModule.GetBlockHeight(context.Background(), srv.URL)
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cosmos/gov/v1/params/voting":
			_, _ = w.Write([]byte(`{"voting_params":{"voting_period":"60s","expedited_voting_period":"30s"}}`))
		case "/cosmos/gov/v1/params/deposit":
			_, _ = w.Write([]byte(`{"deposit_params":{"min_deposit":[{"denom":"uatom","amount":"10000000"}],"expedited_min_deposit":[{"denom":"uatom","amount":"50000000"}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	resp, err := networkModule.GetGovernanceParams(srv.URL, "")
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cosmos/gov/v1/proposals/7" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"proposal":{"id":"7","title":"Upgrade","summary":"desc","metadata":"","status":"PROPOSAL_STATUS_VOTING_PERIOD","submit_time":"2025-01-01T00:00:00Z","deposit_end_time":"2025-01-01T00:10:00Z","voting_end_time":"2025-01-01T00:20:00Z","total_deposit":[{"denom":"uatom","amount":"123"}],"final_tally_result":{"yes_count":"1","no_count":"2","abstain_count":"3"}}}`))
	}))
	defer srv.Close()

	resp, err := networkModule.GetProposal(context.Background(), srv.URL, 7)
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cosmos/upgrade/v1beta1/current_plan" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"plan":null}`))
	}))
	defer srv.Close()

	resp, err := networkModule.GetUpgradePlan(context.Background(), srv.URL)
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
