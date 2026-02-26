package network

import (
	"fmt"
	"strings"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// NormalizeGenesisAccountBalance normalizes account balance inputs from either
// the canonical balance string form or structured coin list form.
// When both are valid and provided, their normalized values must match.
//
// Balance string parsing is best-effort to preserve plugin-level fallback policy:
// invalid balance strings are passed through unchanged when no coin list is present.
func NormalizeGenesisAccountBalance(balance string, coins []Coin) (string, error) {
	parsedList, hasList, err := parseCoinList(coins)
	if err != nil {
		return "", err
	}
	parsedBalance, hasBalance, balanceErr := parseBalanceCoins(balance)

	switch {
	case hasList:
		listStr := coinsToString(parsedList)
		if hasBalance && balanceErr == nil {
			balanceStr := coinsToString(parsedBalance)
			if balanceStr != listStr {
				return "", fmt.Errorf("balance and coins mismatch: balance=%q coins=%q", balanceStr, listStr)
			}
		}
		return listStr, nil
	case hasBalance && balanceErr == nil:
		return coinsToString(parsedBalance), nil
	default:
		// Preserve the original value for downstream plugin fallback handling.
		return strings.TrimSpace(balance), nil
	}
}

// CoinsFromBalance parses a balance string into structured coins.
func CoinsFromBalance(balance string) ([]Coin, error) {
	parsed, hasBalance, err := parseBalanceCoins(balance)
	if err != nil {
		return nil, err
	}
	if !hasBalance {
		return nil, nil
	}
	if len(parsed) == 0 || parsed.IsZero() {
		return nil, nil
	}

	out := make([]Coin, len(parsed))
	for i, coin := range parsed {
		out[i] = Coin{
			Denom:  coin.Denom,
			Amount: coin.Amount.String(),
		}
	}
	return out, nil
}

func parseBalanceCoins(balance string) (sdk.Coins, bool, error) {
	trimmed := strings.TrimSpace(balance)
	if trimmed == "" {
		return nil, false, nil
	}

	parsed, err := sdk.ParseCoinsNormalized(trimmed)
	if err != nil {
		return nil, true, err
	}
	return parsed.Sort(), true, nil
}

func parseCoinList(coins []Coin) (sdk.Coins, bool, error) {
	if len(coins) == 0 {
		return nil, false, nil
	}

	out := sdk.NewCoins()
	hasValue := false
	for idx, coin := range coins {
		denom := strings.TrimSpace(coin.Denom)
		amount := strings.TrimSpace(coin.Amount)

		if denom == "" && amount == "" {
			continue
		}
		hasValue = true
		if denom == "" || amount == "" {
			return nil, true, fmt.Errorf("coin %d requires both denom and amount", idx)
		}

		if err := sdk.ValidateDenom(denom); err != nil {
			return nil, true, fmt.Errorf("coin %d has invalid denom %q: %w", idx, denom, err)
		}

		parsedAmount, ok := math.NewIntFromString(amount)
		if !ok {
			return nil, true, fmt.Errorf("coin %d has invalid amount %q", idx, amount)
		}
		if parsedAmount.IsNegative() {
			return nil, true, fmt.Errorf("coin %d amount must be non-negative", idx)
		}

		out = out.Add(sdk.NewCoin(denom, parsedAmount))
	}

	return out.Sort(), hasValue, nil
}

func coinsToString(coins sdk.Coins) string {
	if len(coins) == 0 || coins.IsZero() {
		return ""
	}
	return coins.Sort().String()
}
