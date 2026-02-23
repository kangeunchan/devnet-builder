package cosmos

import (
	"fmt"
	"strings"

	"github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
)

func errMessage(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func blockHeightError(err error) (*plugin.BlockHeightResponse, error) {
	return &plugin.BlockHeightResponse{Error: errMessage(err)}, nil
}

func blockTimeError(err error) (*plugin.BlockTimeResponse, error) {
	return &plugin.BlockTimeResponse{Error: errMessage(err)}, nil
}

func chainStatusError(err error) (*plugin.ChainStatusResponse, error) {
	return &plugin.ChainStatusResponse{IsRunning: false, Error: errMessage(err)}, nil
}

func waitForBlockError(currentHeight int64, err error) (*plugin.WaitForBlockResponse, error) {
	return &plugin.WaitForBlockResponse{
		CurrentHeight: currentHeight,
		Reached:       false,
		Error:         errMessage(err),
	}, nil
}

func governanceParamsError(err error) (*plugin.GovernanceParamsResponse, error) {
	return &plugin.GovernanceParamsResponse{Error: errMessage(err)}, nil
}

func proposalError(err error) (*plugin.ProposalResponse, error) {
	return &plugin.ProposalResponse{Error: errMessage(err)}, nil
}

func upgradePlanError(err error) (*plugin.UpgradePlanResponse, error) {
	return &plugin.UpgradePlanResponse{Error: errMessage(err)}, nil
}

func appVersionError(err error) (*plugin.AppVersionResponse, error) {
	return &plugin.AppVersionResponse{Error: errMessage(err)}, nil
}

func missingEndpointError(endpointName string) error {
	return fmt.Errorf("%s endpoint is required", endpointName)
}
