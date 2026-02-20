package network

import (
	"strings"
	"testing"
)

func TestNormalizeGenesisAccountBalance_PrefersConsistentInputs(t *testing.T) {
	normalized, err := NormalizeGenesisAccountBalance("2500000uatom", []Coin{
		{Denom: "uatom", Amount: "2500000"},
	})
	if err != nil {
		t.Fatalf("NormalizeGenesisAccountBalance returned error: %v", err)
	}
	if normalized != "2500000uatom" {
		t.Fatalf("unexpected normalized balance: %q", normalized)
	}
}

func TestNormalizeGenesisAccountBalance_MismatchReturnsError(t *testing.T) {
	_, err := NormalizeGenesisAccountBalance("1000000uatom", []Coin{
		{Denom: "uatom", Amount: "2000000"},
	})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeGenesisAccountBalance_InvalidBalancePassesThroughForPluginFallback(t *testing.T) {
	normalized, err := NormalizeGenesisAccountBalance("invalid", nil)
	if err != nil {
		t.Fatalf("NormalizeGenesisAccountBalance returned error: %v", err)
	}
	if normalized != "invalid" {
		t.Fatalf("unexpected normalized balance: %q", normalized)
	}
}

func TestNormalizeGenesisAccountBalance_InvalidBalanceUsesCoinsWhenProvided(t *testing.T) {
	normalized, err := NormalizeGenesisAccountBalance("invalid", []Coin{
		{Denom: "uatom", Amount: "123"},
	})
	if err != nil {
		t.Fatalf("NormalizeGenesisAccountBalance returned error: %v", err)
	}
	if normalized != "123uatom" {
		t.Fatalf("unexpected normalized balance: %q", normalized)
	}
}

func TestCoinsFromBalance_InvalidBalanceReturnsError(t *testing.T) {
	_, err := CoinsFromBalance("invalid")
	if err == nil {
		t.Fatal("expected error for invalid balance")
	}
}
