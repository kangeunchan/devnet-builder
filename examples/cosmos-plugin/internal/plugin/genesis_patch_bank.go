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
	defaultFunding := parseAmountIntOrZero("1000000000")
	validatorFunding, _ := parseCoinAmountWithFallback(n.DefaultGeneratorConfig().ValidatorBalance, cfg.BaseDenom, defaultFunding)
	accountFunding, _ := parseCoinAmountWithFallback(n.DefaultGeneratorConfig().AccountBalance, cfg.BaseDenom, defaultFunding)
	targets := make(map[string]sdkmath.Int, len(validatorAccounts))
	for _, addr := range validatorAccounts {
		targets[addr] = validatorFunding
	}
	for _, account := range addAccounts {
		addr := strings.TrimSpace(account.Address)
		if addr == "" {
			continue
		}
		amount, fallbackUsed := parseCoinAmountWithFallback(account.Balance, cfg.BaseDenom, accountFunding)
		if fallbackUsed && strings.TrimSpace(account.Balance) != "" && n.invalidBalancePolicy() == invalidBalancePolicyError {
			return fmt.Errorf("invalid account balance for %q: %q", addr, account.Balance)
		}
		if current, ok := targets[addr]; !ok || current.LT(amount) {
			targets[addr] = amount
		}
	}

	moduleAddrs := map[string]string{}
	if auth != nil {
		moduleAddrs = genesisinternal.CollectModuleAccountAddresses(auth)
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
