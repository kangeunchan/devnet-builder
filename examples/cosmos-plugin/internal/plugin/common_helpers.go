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

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
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

// parseCoinAmountWithFallback returns the parsed amount and whether fallback was used.
// Fallback is intentional for malformed optional funding inputs in genesis options.
func parseCoinAmountWithFallback(coinStr, expectedDenom string, fallback sdkmath.Int) (sdkmath.Int, bool) {
	coinStr = strings.TrimSpace(coinStr)
	if coinStr == "" {
		return fallback, true
	}

	coin, err := sdk.ParseCoinNormalized(coinStr)
	if err != nil {
		return fallback, true
	}
	if expectedDenom != "" && coin.Denom != expectedDenom {
		return fallback, true
	}
	if coin.Amount.IsNegative() {
		return fallback, true
	}

	return coin.Amount, false
}

func newValidatorDescription(moniker string) map[string]any {
	return map[string]any{
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
