package cosmos

import (
	"context"
	"io"
	"log"
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
	switch strings.TrimSpace(networkType) {
	case "mainnet":
		return fetchFirstMatchingSnapshotURL(mainnetSnapshotIndexURL, mainnetSnapshotURLPattern)
	case "testnet":
		return fetchFirstMatchingSnapshotURL(testnetSnapshotIndexURL, testnetSnapshotURLPattern)
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
		logSnapshotResolverDebug("failed to build snapshot index request url=%q err=%v", indexURL, err)
		return ""
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		logSnapshotResolverDebug("failed to fetch snapshot index url=%q err=%v", indexURL, err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logSnapshotResolverDebug("snapshot index returned non-2xx url=%q status=%d", indexURL, resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logSnapshotResolverDebug("failed to read snapshot index body url=%q err=%v", indexURL, err)
		return ""
	}

	match := urlPattern.Find(body)
	if len(match) == 0 {
		logSnapshotResolverDebug("snapshot link not found in index url=%q", indexURL)
		return ""
	}

	return strings.TrimSpace(string(match))
}

func logSnapshotResolverDebug(format string, args ...any) {
	log.Printf("[cosmos-plugin:snapshot] "+format, args...)
}
