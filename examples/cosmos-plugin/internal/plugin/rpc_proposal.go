package cosmos

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	rpcinternal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/rpc"
	"github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
)

func (n *CosmosNetwork) GetProposal(ctx context.Context, rpcEndpoint string, proposalID uint64) (*plugin.ProposalResponse, error) {
	ctx = ensureContext(ctx)
	ctx, cancel := n.withRequestTimeout(ctx)
	defer cancel()

	rest := n.resolveRESTEndpoint(rpcEndpoint, "")
	if rest == "" {
		return proposalError(missingEndpointError("REST"))
	}

	var result struct {
		Proposal struct {
			ID             string             `json:"id"`
			Title          string             `json:"title"`
			Summary        string             `json:"summary"`
			Metadata       string             `json:"metadata"`
			Status         string             `json:"status"`
			SubmitTime     string             `json:"submit_time"`
			DepositEndTime string             `json:"deposit_end_time"`
			VotingEndTime  string             `json:"voting_end_time"`
			TotalDeposit   []rpcinternal.Coin `json:"total_deposit"`
			FinalTally     struct {
				YesCount     string `json:"yes_count"`
				NoCount      string `json:"no_count"`
				AbstainCount string `json:"abstain_count"`
			} `json:"final_tally_result"`
		} `json:"proposal"`
	}
	if err := n.getJSON(ctx, fmt.Sprintf("%s/cosmos/gov/v1/proposals/%d", rest, proposalID), &result); err != nil {
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
		SubmitTimeUnix:     rpcinternal.ParseRFC3339ToUnixSeconds(result.Proposal.SubmitTime),
		DepositEndTimeUnix: rpcinternal.ParseRFC3339ToUnixSeconds(result.Proposal.DepositEndTime),
		VotingEndTimeUnix:  rpcinternal.ParseRFC3339ToUnixSeconds(result.Proposal.VotingEndTime),
		TotalDeposit:       rpcinternal.FirstCoinAmountString(result.Proposal.TotalDeposit),
		FinalTallyYes:      result.Proposal.FinalTally.YesCount,
		FinalTallyNo:       result.Proposal.FinalTally.NoCount,
		FinalTallyAbstain:  result.Proposal.FinalTally.AbstainCount,
	}, nil
}

func (n *CosmosNetwork) GetUpgradePlan(ctx context.Context, rpcEndpoint string) (*plugin.UpgradePlanResponse, error) {
	ctx = ensureContext(ctx)
	ctx, cancel := n.withRequestTimeout(ctx)
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
	if err := n.getJSON(ctx, rest+"/cosmos/upgrade/v1beta1/current_plan", &result); err != nil {
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
		TimeUnix: rpcinternal.ParseRFC3339ToUnixSeconds(result.Plan.Time),
	}, nil
}
