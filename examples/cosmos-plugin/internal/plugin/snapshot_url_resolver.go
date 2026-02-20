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
	profile, ok := networkProfileByType(networkType)
	if !ok {
		return ""
	}

	switch canonicalNetworkType(networkType) {
	case networkMainnet:
		return fetchFirstMatchingSnapshotURL(profile.SnapshotIndexURL, mainnetSnapshotURLPattern)
	case networkTestnet:
		return fetchFirstMatchingSnapshotURL(profile.SnapshotIndexURL, testnetSnapshotURLPattern)
	default:
		return ""
	}
}

func fetchFirstMatchingSnapshotURL(indexURL string, urlPattern *regexp.Regexp) string {
	if strings.TrimSpace(indexURL) == "" || urlPattern == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), snapshotResolverTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return ""
	}

	resp, err := httpClient.Do(req)
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
