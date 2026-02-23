package cosmos

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
)

// ============================================
// Governance params
// ============================================

func (n *CosmosNetwork) GetGovernanceParams(rpcEndpoint, networkType string) (*plugin.GovernanceParamsResponse, error) {
	ctx, cancel := requestContext()
	defer cancel()

	rest := n.resolveRESTEndpoint(rpcEndpoint, networkType)
	if rest == "" {
		return governanceParamsError(missingEndpointError("REST"))
	}

	var votingResp struct {
		VotingParams struct {
			VotingPeriod          string `json:"voting_period"`
			ExpeditedVotingPeriod string `json:"expedited_voting_period"`
		} `json:"voting_params"`
	}
	if err := getJSON(ctx, rest+"/cosmos/gov/v1/params/voting", &votingResp); err != nil {
		return governanceParamsError(err)
	}

	var depositResp struct {
		DepositParams struct {
			MinDeposit          []coin `json:"min_deposit"`
			ExpeditedMinDeposit []coin `json:"expedited_min_deposit"`
		} `json:"deposit_params"`
	}
	if err := getJSON(ctx, rest+"/cosmos/gov/v1/params/deposit", &depositResp); err != nil {
		return governanceParamsError(err)
	}

	votingPeriodNs, err := parseDurationToNanos(votingResp.VotingParams.VotingPeriod)
	if err != nil {
		return governanceParamsError(fmt.Errorf("invalid voting_period: %w", err))
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
	ctx, cancel := withRequestTimeout(ctx)
	defer cancel()

	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")
	if rpc == "" {
		return blockHeightError(missingEndpointError("RPC"))
	}

	var statusResp struct {
		Result struct {
			SyncInfo struct {
				LatestBlockHeight string `json:"latest_block_height"`
			} `json:"sync_info"`
		} `json:"result"`
	}
	if err := getJSON(ctx, rpc+"/status", &statusResp); err != nil {
		return blockHeightError(err)
	}

	height, err := strconv.ParseInt(statusResp.Result.SyncInfo.LatestBlockHeight, 10, 64)
	if err != nil {
		return blockHeightError(fmt.Errorf("failed to parse latest_block_height: %w", err))
	}

	return &plugin.BlockHeightResponse{Height: height}, nil
}

func (n *CosmosNetwork) GetBlockTime(ctx context.Context, rpcEndpoint string, sampleSize int) (*plugin.BlockTimeResponse, error) {
	ctx = ensureContext(ctx)

	if sampleSize < 2 {
		sampleSize = 10
	}

	heightResp, err := n.GetBlockHeight(ctx, rpcEndpoint)
	if err != nil {
		return blockTimeError(err)
	}
	if heightResp.Error != "" {
		return blockTimeError(errors.New(heightResp.Error))
	}

	current := heightResp.Height
	start := current - int64(sampleSize)
	if start < 1 {
		start = 1
	}

	t1, err := n.getBlockTimestamp(ctx, rpcEndpoint, start)
	if err != nil {
		return blockTimeError(err)
	}
	t2, err := n.getBlockTimestamp(ctx, rpcEndpoint, current)
	if err != nil {
		return blockTimeError(err)
	}

	count := current - start
	if count <= 0 {
		return blockTimeError(fmt.Errorf("insufficient block samples"))
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
		return chainStatusError(err)
	}
	if heightResp.Error != "" {
		return chainStatusError(errors.New(heightResp.Error))
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
				return waitForBlockError(lastHeight, fmt.Errorf("wait canceled"))
			}
			return waitForBlockError(lastHeight, fmt.Errorf("timeout waiting for height %d", targetHeight))
		case <-ticker.C:
			resp, err := n.GetBlockHeight(ctx, rpcEndpoint)
			if err != nil {
				continue
			}
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
	ctx, cancel := withRequestTimeout(ctx)
	defer cancel()

	rest := n.resolveRESTEndpoint(rpcEndpoint, "")
	if rest == "" {
		return proposalError(missingEndpointError("REST"))
	}

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
		return proposalError(err)
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
	ctx, cancel := withRequestTimeout(ctx)
	defer cancel()

	rest := n.resolveRESTEndpoint(rpcEndpoint, "")
	if rest == "" {
		return upgradePlanError(missingEndpointError("REST"))
	}

	var result struct {
		Plan *struct {
			Name   string `json:"name"`
			Height string `json:"height"`
			Info   string `json:"info"`
			Time   string `json:"time"`
		} `json:"plan"`
	}
	if err := getJSON(ctx, rest+"/cosmos/upgrade/v1beta1/current_plan", &result); err != nil {
		return upgradePlanError(err)
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
	ctx, cancel := withRequestTimeout(ctx)
	defer cancel()

	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")
	if rpc == "" {
		return appVersionError(missingEndpointError("RPC"))
	}

	var result struct {
		Result struct {
			Response struct {
				Version string `json:"version"`
			} `json:"response"`
		} `json:"result"`
	}
	if err := getJSON(ctx, rpc+"/abci_info", &result); err != nil {
		return appVersionError(err)
	}

	return &plugin.AppVersionResponse{Version: result.Result.Response.Version}, nil
}

func (n *CosmosNetwork) getBlockTimestamp(ctx context.Context, rpcEndpoint string, height int64) (time.Time, error) {
	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")
	if rpc == "" {
		return time.Time{}, missingEndpointError("RPC")
	}

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

func withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, defaultRequestTimeout)
}
