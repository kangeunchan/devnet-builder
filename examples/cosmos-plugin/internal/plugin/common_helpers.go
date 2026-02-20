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
