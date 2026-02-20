package cosmos

import (
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func collectModuleAccountAddresses(auth map[string]interface{}) map[string]string {
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

func ensureAuthBaseAccounts(auth map[string]interface{}, addresses []string) {
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
		accounts = append(accounts, map[string]interface{}{
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

func extractAccountAddress(raw interface{}) string {
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

func indexBalancesByAddress(balances []interface{}) map[string]map[string]interface{} {
	idx := map[string]map[string]interface{}{}
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

func getOrCreateBalanceEntry(balances *[]interface{}, index map[string]map[string]interface{}, address string) map[string]interface{} {
	if entry, ok := index[address]; ok {
		return entry
	}

	entry := map[string]interface{}{
		"address": address,
		"coins":   []interface{}{},
	}
	*balances = append(*balances, entry)
	index[address] = entry

	return entry
}

func ensureCoinAmount(balance map[string]interface{}, denom string, desired sdkmath.Int) {
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
		coins = append(coins, map[string]interface{}{"denom": denom, "amount": desired.String()})
	}

	balance["coins"] = coins
}

func recomputeSupplyFromBalances(balances []interface{}) []interface{} {
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

func parseCoinEntry(raw map[string]interface{}) (sdk.Coin, bool) {
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

func sdkCoinsToInterfaces(coins sdk.Coins) []interface{} {
	supply := make([]interface{}, 0, len(coins))
	for _, c := range coins {
		supply = append(supply, map[string]interface{}{
			"denom":  c.Denom,
			"amount": c.Amount.String(),
		})
	}
	return supply
}
