package snapshot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	networkMainnet = "mainnet"
	networkTestnet = "testnet"

	DefaultMainnetURLPattern = `https://snapshots\.polkachu\.com/snapshots/cosmos/cosmos_[0-9]+\.tar\.[a-z0-9]+`
	DefaultTestnetURLPattern = `https://snapshots\.polkachu\.com/testnet-snapshots/cosmos/cosmos_[0-9]+\.tar\.[a-z0-9]+`
)

type Profile struct {
	SnapshotIndexURL string
}

func ResolveLatestPolkachuSnapshotURL(
	ctx context.Context,
	networkType string,
	profile Profile,
	timeout time.Duration,
	client *http.Client,
	mainnetPatternRaw string,
	testnetPatternRaw string,
) (string, error) {
	if client == nil {
		client = &http.Client{}
	}

	mainnet, err := compilePattern(mainnetPatternRaw, DefaultMainnetURLPattern)
	if err != nil {
		return "", fmt.Errorf("compile mainnet snapshot pattern: %w", err)
	}
	testnet, err := compilePattern(testnetPatternRaw, DefaultTestnetURLPattern)
	if err != nil {
		return "", fmt.Errorf("compile testnet snapshot pattern: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(networkType)) {
	case networkMainnet:
		return fetchFirstMatchingSnapshotURL(ctx, client, profile.SnapshotIndexURL, mainnet, timeout)
	case networkTestnet:
		return fetchFirstMatchingSnapshotURL(ctx, client, profile.SnapshotIndexURL, testnet, timeout)
	default:
		return "", fmt.Errorf("unsupported network type %q", strings.TrimSpace(networkType))
	}
}

func compilePattern(raw string, fallback string) (*regexp.Regexp, error) {
	pattern := strings.TrimSpace(raw)
	if pattern == "" {
		pattern = fallback
	}
	return regexp.Compile(pattern)
}

func fetchFirstMatchingSnapshotURL(ctx context.Context, client *http.Client, indexURL string, pattern *regexp.Regexp, timeout time.Duration) (string, error) {
	if strings.TrimSpace(indexURL) == "" || pattern == nil {
		return "", fmt.Errorf("snapshot index URL and pattern are required")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok && timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return "", fmt.Errorf("build snapshot index request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request snapshot index %q: %w", indexURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("snapshot index %q returned http %d", indexURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read snapshot index %q: %w", indexURL, err)
	}

	match := pattern.Find(body)
	if len(match) == 0 {
		return "", fmt.Errorf("no snapshot URL matched in %q", indexURL)
	}

	return strings.TrimSpace(string(match)), nil
}
