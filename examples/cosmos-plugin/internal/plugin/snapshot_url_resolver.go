package cosmos

import (
	"context"

	snapshotresolver "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/snapshot"
)

func resolveLatestPolkachuSnapshotURL(ctx context.Context, networkType string, profile NetworkProfileConfig, snapshotCfg SnapshotCustomization) (string, error) {
	return snapshotresolver.ResolveLatestPolkachuSnapshotURL(
		ensureContext(ctx),
		canonicalNetworkType(networkType),
		snapshotresolver.Profile{
			SnapshotIndexURL: profile.SnapshotIndexURL,
		},
		defaultSnapshotResolverTimeout,
		httpClient,
		snapshotCfg.MainnetURLPattern,
		snapshotCfg.TestnetURLPattern,
	)
}
