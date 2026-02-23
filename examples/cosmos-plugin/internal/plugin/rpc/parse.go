package rpc

import (
	"strings"
	"time"
)

type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

func ParseDurationToNanos(s string) (int64, error) {
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

func FirstCoinAmountString(coins []Coin) string {
	if len(coins) == 0 {
		return ""
	}
	return coins[0].Amount
}

func ParseRFC3339ToUnixSeconds(v string) int64 {
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
