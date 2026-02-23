package genesis

import (
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func CollectModuleAccountAddresses(auth map[string]any) map[string]string {
	result := map[string]string{}
	accounts, ok := asSlice(auth["accounts"])
	if !ok {
		return result
	}

	for _, raw := range accounts {
		acc, ok := asMap(raw)
		if !ok {
			continue
		}

		typeURL, _ := acc["@type"].(string)
		if !strings.Contains(typeURL, "ModuleAccount") {
			continue
		}

		name, _ := acc["name"].(string)
		if name == "" {
			continue
		}

		if base, ok := asMap(acc["base_account"]); ok {
			if addr, ok := base["address"].(string); ok && addr != "" {
				result[name] = addr
			}
		}
	}

	return result
}

func EnsureAuthBaseAccounts(auth map[string]any, addresses []string) {
	accounts, _ := asSlice(auth["accounts"])
	existing := map[string]bool{}

	for _, raw := range accounts {
		addr := extractAccountAddress(raw)
		if addr != "" {
			existing[addr] = true
		}
	}

	for _, addr := range addresses {
		if addr == "" || existing[addr] {
			continue
		}
		accounts = append(accounts, map[string]any{
			"@type":          "/cosmos.auth.v1beta1.BaseAccount",
			"address":        addr,
			"pub_key":        nil,
			"account_number": "0",
			"sequence":       "0",
		})
		existing[addr] = true
	}

	auth["accounts"] = accounts
}

func IndexBalancesByAddress(balances []any) map[string]map[string]any {
	idx := map[string]map[string]any{}
	for _, raw := range balances {
		bal, ok := asMap(raw)
		if !ok {
			continue
		}
		addr, _ := bal["address"].(string)
		if addr != "" {
			idx[addr] = bal
		}
	}
	return idx
}

func GetOrCreateBalanceEntry(balances *[]any, index map[string]map[string]any, address string) map[string]any {
	if entry, ok := index[address]; ok {
		return entry
	}

	entry := map[string]any{
		"address": address,
		"coins":   []any{},
	}
	*balances = append(*balances, entry)
	index[address] = entry

	return entry
}

func EnsureCoinAmount(balance map[string]any, denom string, desired sdkmath.Int) {
	coins, _ := asSlice(balance["coins"])
	found := false

	for i, c := range coins {
		coinMap, ok := asMap(c)
		if !ok {
			continue
		}
		if coinMap["denom"] == denom {
			cur := parseAmountIntOrZero(fmt.Sprint(coinMap["amount"]))
			if cur.LT(desired) {
				coinMap["amount"] = desired.String()
			}
			coins[i] = coinMap
			found = true
			break
		}
	}

	if !found {
		coins = append(coins, map[string]any{"denom": denom, "amount": desired.String()})
	}

	balance["coins"] = coins
}

func RecomputeSupplyFromBalances(balances []any) []any {
	totalSupply := sdk.NewCoins()

	for _, raw := range balances {
		bal, ok := asMap(raw)
		if !ok {
			continue
		}
		coins, ok := asSlice(bal["coins"])
		if !ok {
			continue
		}
		for _, c := range coins {
			coinMap, ok := asMap(c)
			if !ok {
				continue
			}
			coin, ok := parseCoinEntry(coinMap)
			if !ok {
				continue
			}
			totalSupply = totalSupply.Add(coin)
		}
	}

	return sdkCoinsToInterfaces(totalSupply.Sort())
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func extractAccountAddress(raw any) string {
	acc, ok := asMap(raw)
	if !ok {
		return ""
	}
	if addr, ok := acc["address"].(string); ok {
		return addr
	}
	if base, ok := asMap(acc["base_account"]); ok {
		if addr, ok := base["address"].(string); ok {
			return addr
		}
	}
	return ""
}

func parseCoinEntry(raw map[string]any) (sdk.Coin, bool) {
	denom := strings.TrimSpace(fmt.Sprint(raw["denom"]))
	if denom == "" {
		return sdk.Coin{}, false
	}
	if err := sdk.ValidateDenom(denom); err != nil {
		return sdk.Coin{}, false
	}

	amount := parseAmountIntOrZero(fmt.Sprint(raw["amount"]))
	return sdk.NewCoin(denom, amount), true
}

func sdkCoinsToInterfaces(coins sdk.Coins) []any {
	supply := make([]any, 0, len(coins))
	for _, c := range coins {
		supply = append(supply, map[string]any{
			"denom":  c.Denom,
			"amount": c.Amount.String(),
		})
	}
	return supply
}

func parseAmountIntOrZero(s string) sdkmath.Int {
	amount, ok := sdkmath.NewIntFromString(strings.TrimSpace(s))
	if !ok || amount.IsNegative() {
		return sdkmath.ZeroInt()
	}
	return amount
}
