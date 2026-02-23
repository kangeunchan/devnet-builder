package cosmos

import (
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	genesisinternal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/genesis"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

func (n *CosmosNetwork) validateGenesisOptions(opts network.GenesisOptions) error {
	return genesisinternal.ValidateOptions(n.shouldRequireValidators(), opts)
}

func (n *CosmosNetwork) patchBankModule(
	bank map[string]any,
	auth map[string]any,
	validatorAccounts []string,
	bondedTotal sdkmath.Int,
	cfg network.GenesisConfig,
	addAccounts []network.GenesisAccountInfo,
) error {
	moduleAddrs := map[string]string{}
	if auth != nil {
		moduleAddrs = genesisinternal.CollectModuleAccountAddresses(auth)
	}
	targets, err := n.buildBankFundingTargets(validatorAccounts, addAccounts, bondedTotal, moduleAddrs, cfg.BaseDenom)
	if err != nil {
		return err
	}

	balances, _ := asSlice(bank["balances"])
	balanceIndex := genesisinternal.IndexBalancesByAddress(balances)

	for addr, amt := range targets {
		entry := genesisinternal.GetOrCreateBalanceEntry(&balances, balanceIndex, addr)
		genesisinternal.EnsureCoinAmount(entry, cfg.BaseDenom, amt)
	}

	if bondedAddr, ok := moduleAddrs["bonded_tokens_pool"]; ok {
		entry := genesisinternal.GetOrCreateBalanceEntry(&balances, balanceIndex, bondedAddr)
		genesisinternal.EnsureCoinAmount(entry, cfg.BaseDenom, bondedTotal)
	}
	if notBondedAddr, ok := moduleAddrs["not_bonded_tokens_pool"]; ok {
		entry := genesisinternal.GetOrCreateBalanceEntry(&balances, balanceIndex, notBondedAddr)
		genesisinternal.EnsureCoinAmount(entry, cfg.BaseDenom, sdkmath.ZeroInt())
	}

	bank["balances"] = balances
	bank["supply"] = genesisinternal.RecomputeSupplyFromBalances(balances)
	return nil
}

func (n *CosmosNetwork) buildBankFundingTargets(
	validatorAccounts []string,
	addAccounts []network.GenesisAccountInfo,
	bondedTotal sdkmath.Int,
	moduleAddrs map[string]string,
	baseDenom string,
) (map[string]sdkmath.Int, error) {
	defaultFunding := parseAmountIntOrZero("1000000000")
	validatorFunding, _ := parseCoinAmountWithFallback(n.DefaultGeneratorConfig().ValidatorBalance, baseDenom, defaultFunding)
	accountFunding, _ := parseCoinAmountWithFallback(n.DefaultGeneratorConfig().AccountBalance, baseDenom, defaultFunding)

	targets := make(map[string]sdkmath.Int, len(validatorAccounts)+len(addAccounts)+2)
	for _, addr := range validatorAccounts {
		trimmed := strings.TrimSpace(addr)
		if trimmed == "" {
			continue
		}
		targets[trimmed] = validatorFunding
	}

	for _, account := range addAccounts {
		addr := strings.TrimSpace(account.Address)
		if addr == "" {
			continue
		}
		amount, fallbackUsed := parseCoinAmountWithFallback(account.Balance, baseDenom, accountFunding)
		if fallbackUsed && strings.TrimSpace(account.Balance) != "" && n.invalidBalancePolicy() == invalidBalancePolicyError {
			return nil, fmt.Errorf("invalid account balance for %q: %q", addr, account.Balance)
		}
		if current, ok := targets[addr]; !ok || current.LT(amount) {
			targets[addr] = amount
		}
	}

	if bondedAddr := strings.TrimSpace(moduleAddrs["bonded_tokens_pool"]); bondedAddr != "" {
		targets[bondedAddr] = bondedTotal
	}
	if notBondedAddr := strings.TrimSpace(moduleAddrs["not_bonded_tokens_pool"]); notBondedAddr != "" {
		targets[notBondedAddr] = sdkmath.ZeroInt()
	}

	return targets, nil
}
