package cosmos

import (
	"fmt"

	rpcinternal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/rpc"
	"github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
)

// GetGovernanceParams returns current governance parameters from the REST API.
func (n *CosmosNetwork) GetGovernanceParams(rpcEndpoint, networkType string) (*plugin.GovernanceParamsResponse, error) {
	ctx, cancel := n.requestContext()
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
	if err := n.getJSON(ctx, rest+"/cosmos/gov/v1/params/voting", &votingResp); err != nil {
		return governanceParamsError(err)
	}

	var depositResp struct {
		DepositParams struct {
			MinDeposit          []rpcinternal.Coin `json:"min_deposit"`
			ExpeditedMinDeposit []rpcinternal.Coin `json:"expedited_min_deposit"`
		} `json:"deposit_params"`
	}
	if err := n.getJSON(ctx, rest+"/cosmos/gov/v1/params/deposit", &depositResp); err != nil {
		return governanceParamsError(err)
	}

	votingPeriodNs, err := rpcinternal.ParseDurationToNanos(votingResp.VotingParams.VotingPeriod)
	if err != nil {
		return governanceParamsError(fmt.Errorf("invalid voting_period: %w", err))
	}
	expeditedVotingNs, _ := rpcinternal.ParseDurationToNanos(votingResp.VotingParams.ExpeditedVotingPeriod)

	minDeposit := rpcinternal.FirstCoinAmountString(depositResp.DepositParams.MinDeposit)
	expeditedMin := rpcinternal.FirstCoinAmountString(depositResp.DepositParams.ExpeditedMinDeposit)

	return &plugin.GovernanceParamsResponse{
		VotingPeriodNs:          votingPeriodNs,
		ExpeditedVotingPeriodNs: expeditedVotingNs,
		MinDeposit:              minDeposit,
		ExpeditedMinDeposit:     expeditedMin,
	}, nil
}
