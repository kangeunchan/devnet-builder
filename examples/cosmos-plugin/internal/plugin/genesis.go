package cosmos

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// ============================================
// Genesis modifiers
// ============================================

func (n *CosmosNetwork) ModifyGenesis(genesis []byte, opts network.GenesisOptions) ([]byte, error) {
	var gen map[string]interface{}
	if err := json.Unmarshal(genesis, &gen); err != nil {
		return nil, fmt.Errorf("failed to parse genesis: %w", err)
	}

	if err := n.applyGenesisMutations(gen, opts); err != nil {
		return nil, err
	}

	out, err := json.MarshalIndent(gen, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified genesis: %w", err)
	}

	return out, nil
}

// ModifyGenesisFile handles large genesis files using file paths.
// It avoids keeping both raw input bytes and output bytes in memory simultaneously.
func (n *CosmosNetwork) ModifyGenesisFile(inputPath, outputPath string, opts network.GenesisOptions) (int64, error) {
	if err := validateGenesisOptions(opts); err != nil {
		return 0, err
	}

	in, err := os.Open(inputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open input genesis %q: %w", inputPath, err)
	}
	defer in.Close()

	dec := json.NewDecoder(bufio.NewReader(in))

	tmpPath := outputPath + ".tmp"
	outFile, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create output temp file %q: %w", tmpPath, err)
	}

	bufw := bufio.NewWriter(outFile)
	if err := n.streamModifyGenesis(dec, bufw, opts); err != nil {
		_ = outFile.Close()
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to stream-modify genesis %q: %w", inputPath, err)
	}
	if err := bufw.Flush(); err != nil {
		_ = outFile.Close()
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to flush modified genesis %q: %w", tmpPath, err)
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to close output file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to move %q to %q: %w", tmpPath, outputPath, err)
	}

	st, err := os.Stat(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to stat output genesis %q: %w", outputPath, err)
	}

	return st.Size(), nil
}

func (n *CosmosNetwork) streamModifyGenesis(dec *json.Decoder, w io.Writer, opts network.GenesisOptions) error {
	start, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read genesis start token: %w", err)
	}
	if d, ok := start.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("genesis root must be an object")
	}

	if _, err := io.WriteString(w, "{"); err != nil {
		return err
	}

	first := true
	cfg := n.GenesisConfig()
	hasChainID := false
	hasValidators := false
	hasAppState := false

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("failed to read genesis field name: %w", err)
		}

		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("invalid genesis field token type %T", keyTok)
		}

		if err := writeObjectKey(w, &first, key); err != nil {
			return err
		}

		switch key {
		case "chain_id":
			hasChainID = true
			if opts.ChainID == "" {
				if err := writeRawJSONValue(dec, w); err != nil {
					return fmt.Errorf("failed to copy chain_id: %w", err)
				}
				continue
			}

			if err := discardJSONValue(dec); err != nil {
				return fmt.Errorf("failed to discard chain_id: %w", err)
			}
			if err := writeJSONValue(w, opts.ChainID); err != nil {
				return fmt.Errorf("failed to write chain_id: %w", err)
			}
		case "validators":
			hasValidators = true
			if err := discardJSONValue(dec); err != nil {
				return fmt.Errorf("failed to discard validators: %w", err)
			}
			if err := writeJSONValue(w, []interface{}{}); err != nil {
				return fmt.Errorf("failed to write validators: %w", err)
			}
		case "app_state":
			hasAppState = true
			if err := n.streamModifyAppState(dec, w, opts, cfg); err != nil {
				return err
			}
		default:
			if err := writeRawJSONValue(dec, w); err != nil {
				return fmt.Errorf("failed to copy field %q: %w", key, err)
			}
		}
	}

	if !hasAppState {
		return fmt.Errorf("genesis missing app_state")
	}

	if opts.ChainID != "" && !hasChainID {
		if err := writeObjectKey(w, &first, "chain_id"); err != nil {
			return err
		}
		if err := writeJSONValue(w, opts.ChainID); err != nil {
			return fmt.Errorf("failed to write chain_id: %w", err)
		}
	}

	if !hasValidators {
		if err := writeObjectKey(w, &first, "validators"); err != nil {
			return err
		}
		if err := writeJSONValue(w, []interface{}{}); err != nil {
			return fmt.Errorf("failed to write validators: %w", err)
		}
	}

	end, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read genesis end token: %w", err)
	}
	if d, ok := end.(json.Delim); !ok || d != '}' {
		return fmt.Errorf("invalid genesis object terminator")
	}

	if _, err := io.WriteString(w, "}"); err != nil {
		return err
	}

	return nil
}

// streamModifyAppState keeps a streaming write path and defers bank emission until
// the end of app_state. This guarantees auth/staking-derived data is available when
// patching bank state without buffering the whole app_state in memory.
func (n *CosmosNetwork) streamModifyAppState(dec *json.Decoder, w io.Writer, opts network.GenesisOptions, cfg network.GenesisConfig) error {
	start, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read app_state start token: %w", err)
	}
	if d, ok := start.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("app_state must be an object")
	}

	if _, err := io.WriteString(w, "{"); err != nil {
		return err
	}

	validatorAccounts, bondedTotal := deriveValidatorAccountsAndBondedTotal(opts.Validators, n.Bech32Prefix())
	first := true

	var authModule map[string]interface{}
	var deferredBank map[string]interface{}
	hasDeferredBank := false

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("failed to read app_state field name: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("invalid app_state field token type %T", keyTok)
		}
		switch key {
		case "gov":
			module, err := readObjectValue(dec)
			if err != nil {
				return fmt.Errorf("failed to decode gov module: %w", err)
			}
			module = ensureMap(module)
			n.patchGovParams(map[string]interface{}{"gov": module}, cfg)

			if err := writeObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := writeJSONValue(w, module); err != nil {
				return fmt.Errorf("failed to write gov module: %w", err)
			}
		case "staking":
			module, err := readObjectValue(dec)
			if err != nil {
				return fmt.Errorf("failed to decode staking module: %w", err)
			}
			module = ensureMap(module)
			actualValidatorAccounts, actualBondedTotal := n.patchStakingState(
				map[string]interface{}{"staking": module}, opts, cfg)
			if actualValidatorAccounts != nil {
				validatorAccounts = actualValidatorAccounts
				bondedTotal = actualBondedTotal
			}

			if err := writeObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := writeJSONValue(w, module); err != nil {
				return fmt.Errorf("failed to write staking module: %w", err)
			}
		case "slashing":
			module, err := readObjectValue(dec)
			if err != nil {
				return fmt.Errorf("failed to decode slashing module: %w", err)
			}
			module = ensureMap(module)
			n.patchSlashingState(map[string]interface{}{"slashing": module})

			if err := writeObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := writeJSONValue(w, module); err != nil {
				return fmt.Errorf("failed to write slashing module: %w", err)
			}
		case "distribution":
			module, err := readObjectValue(dec)
			if err != nil {
				return fmt.Errorf("failed to decode distribution module: %w", err)
			}
			module = ensureMap(module)
			n.patchDistributionState(map[string]interface{}{"distribution": module}, opts)

			if err := writeObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := writeJSONValue(w, module); err != nil {
				return fmt.Errorf("failed to write distribution module: %w", err)
			}
		case "auth":
			module, err := readObjectValue(dec)
			if err != nil {
				return fmt.Errorf("failed to decode auth module: %w", err)
			}
			module = ensureMap(module)
			ensureAuthBaseAccounts(module, validatorAccounts)
			authModule = module

			if err := writeObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := writeJSONValue(w, module); err != nil {
				return fmt.Errorf("failed to write auth module: %w", err)
			}
		case "bank":
			module, err := readObjectValue(dec)
			if err != nil {
				return fmt.Errorf("failed to decode bank module: %w", err)
			}
			deferredBank = ensureMap(module)
			hasDeferredBank = true
		case "genutil":
			module, err := readObjectValue(dec)
			if err != nil {
				return fmt.Errorf("failed to decode genutil module: %w", err)
			}
			module = ensureMap(module)
			module["gen_txs"] = []interface{}{}

			if err := writeObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := writeJSONValue(w, module); err != nil {
				return fmt.Errorf("failed to write genutil module: %w", err)
			}
		default:
			if err := writeObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := writeRawJSONValue(dec, w); err != nil {
				return fmt.Errorf("failed to copy app_state module %q: %w", key, err)
			}
		}
	}

	if hasDeferredBank {
		n.patchBankModule(deferredBank, authModule, validatorAccounts, bondedTotal, cfg)
		if err := writeObjectKey(w, &first, "bank"); err != nil {
			return err
		}
		if err := writeJSONValue(w, deferredBank); err != nil {
			return fmt.Errorf("failed to write bank module: %w", err)
		}
	}

	end, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read app_state end token: %w", err)
	}
	if d, ok := end.(json.Delim); !ok || d != '}' {
		return fmt.Errorf("invalid app_state object terminator")
	}

	_, err = io.WriteString(w, "}")
	return err
}

func writeObjectKey(w io.Writer, first *bool, key string) error {
	if !*first {
		if _, err := io.WriteString(w, ","); err != nil {
			return err
		}
	}
	*first = false

	b, err := json.Marshal(key)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err = io.WriteString(w, ":")
	return err
}

func writeRawJSONValue(dec *json.Decoder, w io.Writer) error {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	_, err := w.Write(raw)
	return err
}

func discardJSONValue(dec *json.Decoder) error {
	var raw json.RawMessage
	return dec.Decode(&raw)
}

func writeJSONValue(w io.Writer, value interface{}) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readObjectValue(dec *json.Decoder) (map[string]interface{}, error) {
	var out map[string]interface{}
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func ensureMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	return in
}

func (n *CosmosNetwork) applyGenesisMutations(gen map[string]interface{}, opts network.GenesisOptions) error {
	if err := validateGenesisOptions(opts); err != nil {
		return err
	}

	if opts.ChainID != "" {
		gen["chain_id"] = opts.ChainID
	}

	appState, ok := asMap(gen["app_state"])
	if !ok {
		return fmt.Errorf("genesis missing app_state")
	}

	cfg := n.GenesisConfig()

	// 1) Governance tuning
	n.patchGovParams(appState, cfg)

	// 2) Staking/state replacement
	validatorAccounts, totalStake := n.patchStakingState(appState, opts, cfg)

	// 3) Auth account sync for validator delegators
	authModule := n.patchAuthState(appState, validatorAccounts)

	// 4) Slashing cleanup
	n.patchSlashingState(appState)

	// 5) Distribution cleanup/reset
	n.patchDistributionState(appState, opts)

	// 6) Bank funding + supply sync
	n.patchBankState(appState, authModule, validatorAccounts, totalStake, cfg)

	// 7) Remove legacy top-level validators and genutil gentxs
	gen["validators"] = []interface{}{}
	if genutil, ok := asMap(appState["genutil"]); ok {
		genutil["gen_txs"] = []interface{}{}
	}

	return nil
}

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

func (n *CosmosNetwork) patchAuthState(appState map[string]interface{}, validatorAccounts []string) map[string]interface{} {
	auth, ok := asMap(appState["auth"])
	if !ok {
		return nil
	}

	ensureAuthBaseAccounts(auth, validatorAccounts)
	return auth
}

func (n *CosmosNetwork) patchBankState(appState map[string]interface{}, auth map[string]interface{}, validatorAccounts []string, bondedTotal sdkmath.Int, cfg network.GenesisConfig) {
	bank, ok := asMap(appState["bank"])
	if !ok {
		return
	}

	n.patchBankModule(bank, auth, validatorAccounts, bondedTotal, cfg)
}

func (n *CosmosNetwork) patchBankModule(bank map[string]interface{}, auth map[string]interface{}, validatorAccounts []string, bondedTotal sdkmath.Int, cfg network.GenesisConfig) {
	defaultFunding := parseAmountIntOrZero("1000000000")
	funding := parseCoinAmountOrFallback(n.DefaultGeneratorConfig().ValidatorBalance, cfg.BaseDenom, defaultFunding)
	targets := make(map[string]sdkmath.Int, len(validatorAccounts))
	for _, addr := range validatorAccounts {
		targets[addr] = funding
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
