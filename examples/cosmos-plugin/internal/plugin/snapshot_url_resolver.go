package cosmos

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type snapshotResolverFunc func(ctx context.Context, networkType string) (string, error)

var (
	mainnetSnapshotURLPattern = regexp.MustCompile(`https://snapshots\.polkachu\.com/snapshots/cosmos/cosmos_[0-9]+\.tar\.[a-z0-9]+`)
	testnetSnapshotURLPattern = regexp.MustCompile(`https://snapshots\.polkachu\.com/testnet-snapshots/cosmos/cosmos_[0-9]+\.tar\.[a-z0-9]+`)
)

func resolveLatestPolkachuSnapshotURL(ctx context.Context, networkType string) (string, error) {
	return resolveLatestPolkachuSnapshotURLWithClient(ctx, networkType, httpClient)
}

func resolveLatestPolkachuSnapshotURLWithClient(ctx context.Context, networkType string, client *http.Client) (string, error) {
	profile, err := requireNetworkProfile(networkType)
	if err != nil {
		return "", err
	}

	if client == nil {
		client = httpClient
	}

	switch canonicalNetworkType(networkType) {
	case networkMainnet:
		return fetchFirstMatchingSnapshotURL(ctx, client, profile.SnapshotIndexURL, mainnetSnapshotURLPattern)
	case networkTestnet:
		return fetchFirstMatchingSnapshotURL(ctx, client, profile.SnapshotIndexURL, testnetSnapshotURLPattern)
	default:
		return "", fmt.Errorf("unsupported network type %q", strings.TrimSpace(networkType))
	}
}

func fetchFirstMatchingSnapshotURL(ctx context.Context, client *http.Client, indexURL string, urlPattern *regexp.Regexp) (string, error) {
	if strings.TrimSpace(indexURL) == "" || urlPattern == nil {
		return "", fmt.Errorf("snapshot index URL and pattern are required")
	}
	if client == nil {
		client = httpClient
	}

	ctx = ensureContext(ctx)
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, snapshotResolverTimeout)
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

	match := urlPattern.Find(body)
	if len(match) == 0 {
		return "", fmt.Errorf("no snapshot URL matched in %q", indexURL)
	}

	return strings.TrimSpace(string(match)), nil
}
