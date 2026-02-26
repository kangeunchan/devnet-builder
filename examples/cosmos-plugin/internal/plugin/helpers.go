package cosmos

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
)

type coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asSlice(v interface{}) ([]interface{}, bool) {
	s, ok := v.([]interface{})
	return s, ok
}

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func durationToSDKString(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return fmt.Sprintf("%ds", int64(d.Seconds()))
}

func parseAmountInt(s string) (sdkmath.Int, bool) {
	amount, ok := sdkmath.NewIntFromString(strings.TrimSpace(s))
	if !ok || amount.IsNegative() {
		return sdkmath.ZeroInt(), false
	}
	return amount, true
}

func parseAmountIntOrZero(s string) sdkmath.Int {
	if amount, ok := parseAmountInt(s); ok {
		return amount
	}
	return sdkmath.ZeroInt()
}

func parseAmountIntOrFallback(s, fallback string) sdkmath.Int {
	if amount, ok := parseAmountInt(s); ok {
		return amount
	}
	return parseAmountIntOrZero(fallback)
}

func parseCoinAmountOrFallback(coinStr, expectedDenom string, fallback sdkmath.Int) sdkmath.Int {
	coinStr = strings.TrimSpace(coinStr)
	if coinStr == "" {
		return fallback
	}

	coin, err := sdk.ParseCoinNormalized(coinStr)
	if err != nil {
		return fallback
	}
	if expectedDenom != "" && coin.Denom != expectedDenom {
		return fallback
	}
	if coin.Amount.IsNegative() {
		return fallback
	}

	return coin.Amount
}

func newValidatorDescription(moniker string) map[string]interface{} {
	return map[string]interface{}{
		"moniker":          moniker,
		"identity":         "",
		"website":          "",
		"security_contact": "",
		"details":          "",
	}
}

func valoperToAccount(valoperAddr, bech32Prefix string) (string, error) {
	hrp, data, err := bech32.DecodeAndConvert(valoperAddr)
	if err != nil {
		return "", err
	}

	expected := bech32Prefix + "valoper"
	if hrp != expected {
		return "", fmt.Errorf("unexpected valoper prefix: got %s, expected %s", hrp, expected)
	}

	return bech32.ConvertAndEncode(bech32Prefix, data)
}

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

func parseDurationToNanos(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return d.Nanoseconds(), nil
}

func firstCoinAmountString(coins []coin) string {
	if len(coins) == 0 {
		return ""
	}
	return coins[0].Amount
}

func parseRFC3339ToUnixSeconds(v string) int64 {
	if v == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t.Unix()
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.Unix()
	}
	return 0
}
