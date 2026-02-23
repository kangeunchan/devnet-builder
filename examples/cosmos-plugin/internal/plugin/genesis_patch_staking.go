package cosmos

import (
	"fmt"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

func (n *CosmosNetwork) patchGovParams(appState map[string]any, cfg network.GenesisConfig) {
	gov, ok := asMap(appState[appModuleGov])
	if !ok {
		return
	}

	voting := durationToSDKString(cfg.VotingPeriod)
	deposit := durationToSDKString(cfg.MaxDepositPeriod)

	if params, ok := asMap(gov["params"]); ok {
		params["voting_period"] = voting
		params["max_deposit_period"] = deposit
		if _, exists := params["deposit_period"]; exists {
			params["deposit_period"] = deposit
		}
	}

	if votingParams, ok := asMap(gov["voting_params"]); ok {
		votingParams["voting_period"] = voting
	}
	if depositParams, ok := asMap(gov["deposit_params"]); ok {
		depositParams["max_deposit_period"] = deposit
	}

	gov["proposals"] = []any{}
	gov["votes"] = []any{}
	gov["deposits"] = []any{}
}

func (n *CosmosNetwork) patchStakingState(appState map[string]any, opts network.GenesisOptions, cfg network.GenesisConfig) ([]string, sdkmath.Int) {
	staking, ok := asMap(appState[appModuleStaking])
	if !ok {
		return nil, sdkmath.ZeroInt()
	}

	if params, ok := asMap(staking["params"]); ok {
		params["unbonding_time"] = durationToSDKString(cfg.UnbondingTime)
		if cfg.MaxValidators > 0 {
			params["max_validators"] = fmt.Sprintf("%d", cfg.MaxValidators)
		}
		if cfg.BondDenom != "" {
			params["bond_denom"] = cfg.BondDenom
		}
	}

	if len(opts.Validators) == 0 {
		return nil, sdkmath.ZeroInt()
	}

	validators := make([]any, 0, len(opts.Validators))
	delegations := make([]any, 0, len(opts.Validators))
	lastPowers := make([]any, 0, len(opts.Validators))
	validatorAccounts := make([]string, 0, len(opts.Validators))
	bondedTotal := sdkmath.ZeroInt()
	updateTime := time.Now().UTC().Format(time.RFC3339Nano)

	minSelfDelegation := cfg.MinSelfDelegation
	if minSelfDelegation == "" {
		minSelfDelegation = "1"
	}

	for i, v := range opts.Validators {
		tokens := parseAmountIntOrFallback(v.SelfDelegation, "1000000")
		tokensStr := tokens.String()
		bondedTotal = bondedTotal.Add(tokens)

		moniker := v.Moniker
		if moniker == "" {
			moniker = fmt.Sprintf("validator-%d", i)
		}

		validators = append(validators, map[string]any{
			"operator_address": v.OperatorAddress,
			"consensus_pubkey": map[string]any{
				"@type": "/cosmos.crypto.ed25519.PubKey",
				"key":   v.ConsPubKey,
			},
			"jailed":              false,
			"status":              "BOND_STATUS_BONDED",
			"tokens":              tokensStr,
			"delegator_shares":    tokensStr + decimalPrecision18,
			"description":         newValidatorDescription(moniker),
			"unbonding_height":    "0",
			"unbonding_time":      "1970-01-01T00:00:00Z",
			"min_self_delegation": minSelfDelegation,
			"commission": map[string]any{
				"commission_rates": map[string]any{
					"rate":            "0.100000000000000000",
					"max_rate":        "0.200000000000000000",
					"max_change_rate": "0.010000000000000000",
				},
				"update_time": updateTime,
			},
		})

		delegatorAddr, err := valoperToAccount(v.OperatorAddress, n.Bech32Prefix())
		if err == nil && delegatorAddr != "" {
			validatorAccounts = append(validatorAccounts, delegatorAddr)
			delegations = append(delegations, map[string]any{
				"delegator_address": delegatorAddr,
				"validator_address": v.OperatorAddress,
				"shares":            tokensStr + decimalPrecision18,
			})
		}

		lastPowers = append(lastPowers, map[string]any{
			"address": v.OperatorAddress,
			"power":   tokensStr,
		})
	}

	staking["validators"] = validators
	staking["delegations"] = delegations
	staking["unbonding_delegations"] = []any{}
	staking["redelegations"] = []any{}
	staking["last_validator_powers"] = lastPowers
	staking["last_total_power"] = bondedTotal.String()

	if pool, ok := asMap(staking["pool"]); ok {
		pool["bonded_tokens"] = bondedTotal.String()
		pool["not_bonded_tokens"] = "0"
	}

	return validatorAccounts, bondedTotal
}

func deriveValidatorAccountsAndBondedTotal(validators []network.ValidatorInfo, bech32Prefix string) ([]string, sdkmath.Int) {
	validatorAccounts := make([]string, 0, len(validators))
	bondedTotal := sdkmath.ZeroInt()

	for _, v := range validators {
		tokens := parseAmountIntOrFallback(v.SelfDelegation, "1000000")
		bondedTotal = bondedTotal.Add(tokens)

		delegatorAddr, err := valoperToAccount(v.OperatorAddress, bech32Prefix)
		if err == nil && delegatorAddr != "" {
			validatorAccounts = append(validatorAccounts, delegatorAddr)
		}
	}

	return validatorAccounts, bondedTotal
}

func collectGenesisAccountAddresses(accounts []network.GenesisAccountInfo) []string {
	if len(accounts) == 0 {
		return nil
	}

	addresses := make([]string, 0, len(accounts))
	for _, account := range accounts {
		addr := strings.TrimSpace(account.Address)
		if addr == "" {
			continue
		}
		addresses = append(addresses, addr)
	}
	return addresses
}

func mergeUniqueAddresses(primary []string, secondary []string) []string {
	if len(primary) == 0 && len(secondary) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(primary)+len(secondary))
	merged := make([]string, 0, len(primary)+len(secondary))

	for _, address := range primary {
		addr := strings.TrimSpace(address)
		if addr == "" {
			continue
		}
		if _, exists := seen[addr]; exists {
			continue
		}
		seen[addr] = struct{}{}
		merged = append(merged, addr)
	}

	for _, address := range secondary {
		addr := strings.TrimSpace(address)
		if addr == "" {
			continue
		}
		if _, exists := seen[addr]; exists {
			continue
		}
		seen[addr] = struct{}{}
		merged = append(merged, addr)
	}

	return merged
}
