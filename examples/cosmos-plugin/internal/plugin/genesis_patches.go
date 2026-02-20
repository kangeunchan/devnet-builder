package cosmos

import (
	"fmt"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

func (n *CosmosNetwork) patchGovParams(appState map[string]interface{}, cfg network.GenesisConfig) {
	gov, ok := asMap(appState["gov"])
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

	gov["proposals"] = []interface{}{}
	gov["votes"] = []interface{}{}
	gov["deposits"] = []interface{}{}
}

func (n *CosmosNetwork) patchStakingState(appState map[string]interface{}, opts network.GenesisOptions, cfg network.GenesisConfig) ([]string, sdkmath.Int) {
	staking, ok := asMap(appState["staking"])
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

	validators := make([]interface{}, 0, len(opts.Validators))
	delegations := make([]interface{}, 0, len(opts.Validators))
	lastPowers := make([]interface{}, 0, len(opts.Validators))
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

		validators = append(validators, map[string]interface{}{
			"operator_address": v.OperatorAddress,
			"consensus_pubkey": map[string]interface{}{
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
			"commission": map[string]interface{}{
				"commission_rates": map[string]interface{}{
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
			delegations = append(delegations, map[string]interface{}{
				"delegator_address": delegatorAddr,
				"validator_address": v.OperatorAddress,
				"shares":            tokensStr + decimalPrecision18,
			})
		}

		lastPowers = append(lastPowers, map[string]interface{}{
			"address": v.OperatorAddress,
			"power":   tokensStr,
		})
	}

	staking["validators"] = validators
	staking["delegations"] = delegations
	staking["unbonding_delegations"] = []interface{}{}
	staking["redelegations"] = []interface{}{}
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

func (n *CosmosNetwork) patchSlashingState(appState map[string]interface{}) {
	slashing, ok := asMap(appState["slashing"])
	if !ok {
		return
	}

	slashing["signing_infos"] = []interface{}{}
	slashing["missed_blocks"] = []interface{}{}
}

func (n *CosmosNetwork) patchDistributionState(appState map[string]interface{}, opts network.GenesisOptions) {
	distribution, ok := asMap(appState["distribution"])
	if !ok {
		return
	}

	outstanding := make([]interface{}, 0, len(opts.Validators))
	accumulated := make([]interface{}, 0, len(opts.Validators))
	historical := make([]interface{}, 0, len(opts.Validators))
	current := make([]interface{}, 0, len(opts.Validators))

	for _, v := range opts.Validators {
		op := v.OperatorAddress
		outstanding = append(outstanding, map[string]interface{}{
			"validator_address":   op,
			"outstanding_rewards": []interface{}{},
		})
		accumulated = append(accumulated, map[string]interface{}{
			"validator_address": op,
			"accumulated": map[string]interface{}{
				"commission": []interface{}{},
			},
		})
		historical = append(historical, map[string]interface{}{
			"validator_address": op,
			"rewards": map[string]interface{}{
				"cumulative_reward_ratio": []interface{}{},
				"reference_count":         "1",
			},
		})
		current = append(current, map[string]interface{}{
			"validator_address": op,
			"rewards": map[string]interface{}{
				"rewards": []interface{}{},
				"period":  "1",
			},
		})
	}

	distribution["delegator_starting_infos"] = []interface{}{}
	distribution["validator_slash_events"] = []interface{}{}
	distribution["previous_proposer"] = ""
	distribution["outstanding_rewards"] = outstanding
	distribution["validator_accumulated_commissions"] = accumulated
	distribution["validator_historical_rewards"] = historical
	distribution["validator_current_rewards"] = current
}

func validateGenesisOptions(opts network.GenesisOptions) error {
	if len(opts.Validators) == 0 {
		return fmt.Errorf("genesis options validators are required for validator-set replacement")
	}
	return nil
}

func (n *CosmosNetwork) patchBankModule(bank map[string]interface{}, auth map[string]interface{}, validatorAccounts []string, bondedTotal sdkmath.Int, cfg network.GenesisConfig, addAccounts []network.GenesisAccountInfo) {
	defaultFunding := parseAmountIntOrZero("1000000000")
	validatorFunding := parseCoinAmountOrFallback(n.DefaultGeneratorConfig().ValidatorBalance, cfg.BaseDenom, defaultFunding)
	accountFunding := parseCoinAmountOrFallback(n.DefaultGeneratorConfig().AccountBalance, cfg.BaseDenom, defaultFunding)
	targets := make(map[string]sdkmath.Int, len(validatorAccounts))
	for _, addr := range validatorAccounts {
		targets[addr] = validatorFunding
	}
	for _, account := range addAccounts {
		addr := strings.TrimSpace(account.Address)
		if addr == "" {
			continue
		}
		amount := parseCoinAmountOrFallback(account.Balance, cfg.BaseDenom, accountFunding)
		if current, ok := targets[addr]; !ok || current.LT(amount) {
			targets[addr] = amount
		}
	}

	moduleAddrs := map[string]string{}
	if auth != nil {
		moduleAddrs = collectModuleAccountAddresses(auth)
	}

	balances, _ := asSlice(bank["balances"])
	balanceIndex := indexBalancesByAddress(balances)

	for addr, amt := range targets {
		entry := getOrCreateBalanceEntry(&balances, balanceIndex, addr)
		ensureCoinAmount(entry, cfg.BaseDenom, amt)
	}

	if bondedAddr, ok := moduleAddrs["bonded_tokens_pool"]; ok {
		entry := getOrCreateBalanceEntry(&balances, balanceIndex, bondedAddr)
		ensureCoinAmount(entry, cfg.BaseDenom, bondedTotal)
	}
	if notBondedAddr, ok := moduleAddrs["not_bonded_tokens_pool"]; ok {
		entry := getOrCreateBalanceEntry(&balances, balanceIndex, notBondedAddr)
		ensureCoinAmount(entry, cfg.BaseDenom, sdkmath.ZeroInt())
	}

	bank["balances"] = balances
	bank["supply"] = recomputeSupplyFromBalances(balances)
}
