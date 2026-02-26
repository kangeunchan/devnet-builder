package cosmos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
)

// ============================================
// Governance params
// ============================================

func (n *CosmosNetwork) GetGovernanceParams(rpcEndpoint, networkType string) (*plugin.GovernanceParamsResponse, error) {
	rest := n.resolveRESTEndpoint(rpcEndpoint, networkType)

	var votingResp struct {
		VotingParams struct {
			VotingPeriod          string `json:"voting_period"`
			ExpeditedVotingPeriod string `json:"expedited_voting_period"`
		} `json:"voting_params"`
	}
	if err := getJSON(context.Background(), rest+"/cosmos/gov/v1/params/voting", &votingResp); err != nil {
		return &plugin.GovernanceParamsResponse{Error: err.Error()}, nil
	}

	var depositResp struct {
		DepositParams struct {
			MinDeposit          []coin `json:"min_deposit"`
			ExpeditedMinDeposit []coin `json:"expedited_min_deposit"`
		} `json:"deposit_params"`
	}
	if err := getJSON(context.Background(), rest+"/cosmos/gov/v1/params/deposit", &depositResp); err != nil {
		return &plugin.GovernanceParamsResponse{Error: err.Error()}, nil
	}

	votingPeriodNs, err := parseDurationToNanos(votingResp.VotingParams.VotingPeriod)
	if err != nil {
		return &plugin.GovernanceParamsResponse{Error: fmt.Sprintf("invalid voting_period: %v", err)}, nil
	}
	expeditedVotingNs, _ := parseDurationToNanos(votingResp.VotingParams.ExpeditedVotingPeriod)

	minDeposit := firstCoinAmountString(depositResp.DepositParams.MinDeposit)
	expeditedMin := firstCoinAmountString(depositResp.DepositParams.ExpeditedMinDeposit)

	return &plugin.GovernanceParamsResponse{
		VotingPeriodNs:          votingPeriodNs,
		ExpeditedVotingPeriodNs: expeditedVotingNs,
		MinDeposit:              minDeposit,
		ExpeditedMinDeposit:     expeditedMin,
	}, nil
}

// ============================================
// RPC operations
// ============================================

func (n *CosmosNetwork) GetBlockHeight(ctx context.Context, rpcEndpoint string) (*plugin.BlockHeightResponse, error) {
	ctx = ensureContext(ctx)

	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")

	var statusResp struct {
		Result struct {
			SyncInfo struct {
				LatestBlockHeight string `json:"latest_block_height"`
			} `json:"sync_info"`
		} `json:"result"`
	}
	if err := getJSON(ctx, rpc+"/status", &statusResp); err != nil {
		return &plugin.BlockHeightResponse{Error: err.Error()}, nil
	}

	height, err := strconv.ParseInt(statusResp.Result.SyncInfo.LatestBlockHeight, 10, 64)
	if err != nil {
		return &plugin.BlockHeightResponse{Error: fmt.Sprintf("failed to parse latest_block_height: %v", err)}, nil
	}

	return &plugin.BlockHeightResponse{Height: height}, nil
}

func (n *CosmosNetwork) GetBlockTime(ctx context.Context, rpcEndpoint string, sampleSize int) (*plugin.BlockTimeResponse, error) {
	ctx = ensureContext(ctx)

	if sampleSize < 2 {
		sampleSize = 10
	}

	heightResp, _ := n.GetBlockHeight(ctx, rpcEndpoint)
	if heightResp.Error != "" {
		return &plugin.BlockTimeResponse{Error: heightResp.Error}, nil
	}

	current := heightResp.Height
	start := current - int64(sampleSize)
	if start < 1 {
		start = 1
	}

	t1, err := n.getBlockTimestamp(ctx, rpcEndpoint, start)
	if err != nil {
		return &plugin.BlockTimeResponse{Error: err.Error()}, nil
	}
	t2, err := n.getBlockTimestamp(ctx, rpcEndpoint, current)
	if err != nil {
		return &plugin.BlockTimeResponse{Error: err.Error()}, nil
	}

	count := current - start
	if count <= 0 {
		return &plugin.BlockTimeResponse{Error: "insufficient block samples"}, nil
	}
	avg := t2.Sub(t1) / time.Duration(count)
	if avg <= 0 {
		avg = defaultBlockTime
	}

	return &plugin.BlockTimeResponse{BlockTimeNs: int64(avg)}, nil
}

func (n *CosmosNetwork) IsChainRunning(ctx context.Context, rpcEndpoint string) (*plugin.ChainStatusResponse, error) {
	ctx = ensureContext(ctx)

	heightResp, err := n.GetBlockHeight(ctx, rpcEndpoint)
	if err != nil {
		return &plugin.ChainStatusResponse{IsRunning: false, Error: err.Error()}, nil
	}
	if heightResp.Error != "" {
		return &plugin.ChainStatusResponse{IsRunning: false, Error: heightResp.Error}, nil
	}

	return &plugin.ChainStatusResponse{IsRunning: true}, nil
}

func (n *CosmosNetwork) WaitForBlock(ctx context.Context, rpcEndpoint string, targetHeight int64, timeoutMs int64) (*plugin.WaitForBlockResponse, error) {
	ctx = ensureContext(ctx)

	timeout := defaultWaitTimeout
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(blockPollInterval)
	defer ticker.Stop()

	var lastHeight int64
	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if errors.Is(err, context.Canceled) {
				return &plugin.WaitForBlockResponse{
					CurrentHeight: lastHeight,
					Reached:       false,
					Error:         "wait canceled",
				}, nil
			}
			return &plugin.WaitForBlockResponse{
				CurrentHeight: lastHeight,
				Reached:       false,
				Error:         fmt.Sprintf("timeout waiting for height %d", targetHeight),
			}, nil
		case <-ticker.C:
			resp, _ := n.GetBlockHeight(ctx, rpcEndpoint)
			if resp == nil || resp.Error != "" {
				continue
			}
			lastHeight = resp.Height
			if resp.Height >= targetHeight {
				return &plugin.WaitForBlockResponse{CurrentHeight: resp.Height, Reached: true}, nil
			}
		}
	}
}

func (n *CosmosNetwork) GetProposal(ctx context.Context, rpcEndpoint string, proposalID uint64) (*plugin.ProposalResponse, error) {
	ctx = ensureContext(ctx)

	rest := n.resolveRESTEndpoint(rpcEndpoint, "")

	var result struct {
		Proposal struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Summary        string `json:"summary"`
			Metadata       string `json:"metadata"`
			Status         string `json:"status"`
			SubmitTime     string `json:"submit_time"`
			DepositEndTime string `json:"deposit_end_time"`
			VotingEndTime  string `json:"voting_end_time"`
			TotalDeposit   []coin `json:"total_deposit"`
			FinalTally     struct {
				YesCount     string `json:"yes_count"`
				NoCount      string `json:"no_count"`
				AbstainCount string `json:"abstain_count"`
			} `json:"final_tally_result"`
		} `json:"proposal"`
	}
	if err := getJSON(ctx, fmt.Sprintf("%s/cosmos/gov/v1/proposals/%d", rest, proposalID), &result); err != nil {
		return &plugin.ProposalResponse{Error: err.Error()}, nil
	}

	title := strings.TrimSpace(result.Proposal.Title)
	if title == "" {
		title = strings.TrimSpace(result.Proposal.Metadata)
	}
	desc := strings.TrimSpace(result.Proposal.Summary)

	return &plugin.ProposalResponse{
		Id:                 proposalID,
		Title:              title,
		Description:        desc,
		Status:             result.Proposal.Status,
		SubmitTimeUnix:     parseRFC3339ToUnixSeconds(result.Proposal.SubmitTime),
		DepositEndTimeUnix: parseRFC3339ToUnixSeconds(result.Proposal.DepositEndTime),
		VotingEndTimeUnix:  parseRFC3339ToUnixSeconds(result.Proposal.VotingEndTime),
		TotalDeposit:       firstCoinAmountString(result.Proposal.TotalDeposit),
		FinalTallyYes:      result.Proposal.FinalTally.YesCount,
		FinalTallyNo:       result.Proposal.FinalTally.NoCount,
		FinalTallyAbstain:  result.Proposal.FinalTally.AbstainCount,
	}, nil
}

func (n *CosmosNetwork) GetUpgradePlan(ctx context.Context, rpcEndpoint string) (*plugin.UpgradePlanResponse, error) {
	ctx = ensureContext(ctx)

	rest := n.resolveRESTEndpoint(rpcEndpoint, "")

	var result struct {
		Plan *struct {
			Name   string `json:"name"`
			Height string `json:"height"`
			Info   string `json:"info"`
			Time   string `json:"time"`
		} `json:"plan"`
	}
	if err := getJSON(ctx, rest+"/cosmos/upgrade/v1beta1/current_plan", &result); err != nil {
		return &plugin.UpgradePlanResponse{Error: err.Error()}, nil
	}

	if result.Plan == nil {
		return &plugin.UpgradePlanResponse{HasPlan: false}, nil
	}

	height, _ := strconv.ParseInt(result.Plan.Height, 10, 64)
	return &plugin.UpgradePlanResponse{
		HasPlan:  true,
		Name:     result.Plan.Name,
		Height:   height,
		Info:     result.Plan.Info,
		TimeUnix: parseRFC3339ToUnixSeconds(result.Plan.Time),
	}, nil
}

func (n *CosmosNetwork) GetAppVersion(ctx context.Context, rpcEndpoint string) (*plugin.AppVersionResponse, error) {
	ctx = ensureContext(ctx)

	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")

	var result struct {
		Result struct {
			Response struct {
				Version string `json:"version"`
			} `json:"response"`
		} `json:"result"`
	}
	if err := getJSON(ctx, rpc+"/abci_info", &result); err != nil {
		return &plugin.AppVersionResponse{Error: err.Error()}, nil
	}

	return &plugin.AppVersionResponse{Version: result.Result.Response.Version}, nil
}

func (n *CosmosNetwork) resolveRPCEndpoint(raw, networkType string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		if networkType == "testnet" {
			return testnetRPC
		}
		return mainnetRPC
	}

	endpoint = normalizeEndpoint(endpoint)

	if strings.Contains(endpoint, "cosmos-api.polkachu.com") {
		return strings.Replace(endpoint, "cosmos-api.polkachu.com", "cosmos-rpc.polkachu.com", 1)
	}
	if strings.Contains(endpoint, "cosmos-testnet-api.polkachu.com") {
		return strings.Replace(endpoint, "cosmos-testnet-api.polkachu.com", "cosmos-testnet-rpc.polkachu.com", 1)
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err == nil {
		if port == "1317" {
			u.Host = net.JoinHostPort(host, "26657")
		}
		return strings.TrimRight(u.String(), "/")
	}

	if isLocalHost(u.Host) {
		u.Host = net.JoinHostPort(u.Host, "26657")
	}

	return strings.TrimRight(u.String(), "/")
}

func (n *CosmosNetwork) resolveRESTEndpoint(raw, networkType string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		if networkType == "testnet" {
			return testnetREST
		}
		return mainnetREST
	}

	endpoint = normalizeEndpoint(endpoint)

	if strings.Contains(endpoint, "cosmos-rpc.polkachu.com") {
		return strings.Replace(endpoint, "cosmos-rpc.polkachu.com", "cosmos-api.polkachu.com", 1)
	}
	if strings.Contains(endpoint, "cosmos-testnet-rpc.polkachu.com") {
		return strings.Replace(endpoint, "cosmos-testnet-rpc.polkachu.com", "cosmos-testnet-api.polkachu.com", 1)
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err == nil {
		if port == "26657" {
			u.Host = net.JoinHostPort(host, "1317")
		}
		return strings.TrimRight(u.String(), "/")
	}

	if isLocalHost(u.Host) {
		u.Host = net.JoinHostPort(u.Host, "1317")
	}

	return strings.TrimRight(u.String(), "/")
}

func normalizeEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return strings.TrimRight(raw, "/")
}

func isLocalHost(host string) bool {
	h := host
	if strings.Contains(h, ":") {
		if parsedHost, _, err := net.SplitHostPort(h); err == nil {
			h = parsedHost
		}
	}
	return h == "localhost" || h == "127.0.0.1"
}

func (n *CosmosNetwork) getBlockTimestamp(ctx context.Context, rpcEndpoint string, height int64) (time.Time, error) {
	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")

	var resp struct {
		Result struct {
			Block struct {
				Header struct {
					Time string `json:"time"`
				} `json:"header"`
			} `json:"block"`
		} `json:"result"`
	}
	if err := getJSON(ctx, fmt.Sprintf("%s/block?height=%d", rpc, height), &resp); err != nil {
		return time.Time{}, err
	}
	if resp.Result.Block.Header.Time == "" {
		return time.Time{}, fmt.Errorf("missing block time for height %d", height)
	}

	if t, err := time.Parse(time.RFC3339Nano, resp.Result.Block.Header.Time); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, resp.Result.Block.Header.Time); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("failed to parse block time %q", resp.Result.Block.Header.Time)
}

func getJSON(ctx context.Context, endpoint string, out interface{}) error {
	ctx = ensureContext(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil {
		return err
	}

	return nil
}
