package ports

import "context"

// ExportHashCalculator calculates hashes for exported artifacts.
type ExportHashCalculator interface {
	CalculateHash(binaryPath string) (string, error)
}

// ExportHeightResolver resolves current block height from an RPC endpoint.
type ExportHeightResolver interface {
	GetCurrentHeight(ctx context.Context, rpcURL string) (int64, error)
}

// ExportExecutor executes binary export commands.
type ExportExecutor interface {
	GetBinaryVersion(ctx context.Context, binaryPath string) (string, error)
	ExportAtHeight(ctx context.Context, binaryPath, homeDir string, height int64, outputPath string) (string, error)
}
