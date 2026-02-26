package cosmos

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type snapshotResolverFunc func(networkType string) string

var (
	mainnetSnapshotURLPattern = regexp.MustCompile(`https://snapshots\.polkachu\.com/snapshots/cosmos/cosmos_[0-9]+\.tar\.[a-z0-9]+`)
	testnetSnapshotURLPattern = regexp.MustCompile(`https://snapshots\.polkachu\.com/testnet-snapshots/cosmos/cosmos_[0-9]+\.tar\.[a-z0-9]+`)
)

func resolveLatestPolkachuSnapshotURL(networkType string) string {
	return ResolveLatestPolkachuSnapshotURLWithClient(networkType, httpClient)
}

// ResolveLatestPolkachuSnapshotURLWithClient resolves the latest snapshot URL
// from Polkachu index pages for the given network type using the provided HTTP client.
// This is exported so unit tests outside this package can validate resolver behavior.
func ResolveLatestPolkachuSnapshotURLWithClient(networkType string, client *http.Client) string {
	profile, ok := networkProfileByType(networkType)
	if !ok {
		return ""
	}

	switch canonicalNetworkType(networkType) {
	case networkMainnet:
		return fetchFirstMatchingSnapshotURL(client, profile.SnapshotIndexURL, mainnetSnapshotURLPattern)
	case networkTestnet:
		return fetchFirstMatchingSnapshotURL(client, profile.SnapshotIndexURL, testnetSnapshotURLPattern)
	default:
		return ""
	}
}

func fetchFirstMatchingSnapshotURL(client *http.Client, indexURL string, urlPattern *regexp.Regexp) string {
	if strings.TrimSpace(indexURL) == "" || urlPattern == nil {
		return ""
	}
	if client == nil {
		client = httpClient
	}

	ctx, cancel := context.WithTimeout(context.Background(), snapshotResolverTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return ""
	}

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	match := urlPattern.Find(body)
	if len(match) == 0 {
		return ""
	}

	return strings.TrimSpace(string(match))
}
