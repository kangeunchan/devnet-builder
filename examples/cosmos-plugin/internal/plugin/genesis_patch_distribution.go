package cosmos

import "github.com/altuslabsxyz/devnet-builder/pkg/network"

func (n *CosmosNetwork) patchSlashingState(appState map[string]any) {
	slashing, ok := asMap(appState[appModuleSlashing])
	if !ok {
		return
	}

	slashing["signing_infos"] = []any{}
	slashing["missed_blocks"] = []any{}
}

func (n *CosmosNetwork) patchDistributionState(appState map[string]any, opts network.GenesisOptions) {
	distribution, ok := asMap(appState[appModuleDistribution])
	if !ok {
		return
	}

	outstanding := make([]any, 0, len(opts.Validators))
	accumulated := make([]any, 0, len(opts.Validators))
	historical := make([]any, 0, len(opts.Validators))
	current := make([]any, 0, len(opts.Validators))

	for _, v := range opts.Validators {
		op := v.OperatorAddress
		outstanding = append(outstanding, map[string]any{
			"validator_address":   op,
			"outstanding_rewards": []any{},
		})
		accumulated = append(accumulated, map[string]any{
			"validator_address": op,
			"accumulated": map[string]any{
				"commission": []any{},
			},
		})
		historical = append(historical, map[string]any{
			"validator_address": op,
			"rewards": map[string]any{
				"cumulative_reward_ratio": []any{},
				"reference_count":         "1",
			},
		})
		current = append(current, map[string]any{
			"validator_address": op,
			"rewards": map[string]any{
				"rewards": []any{},
				"period":  "1",
			},
		})
	}

	distribution["delegator_starting_infos"] = []any{}
	distribution["validator_slash_events"] = []any{}
	distribution["previous_proposer"] = ""
	distribution["outstanding_rewards"] = outstanding
	distribution["validator_accumulated_commissions"] = accumulated
	distribution["validator_historical_rewards"] = historical
	distribution["validator_current_rewards"] = current
}
