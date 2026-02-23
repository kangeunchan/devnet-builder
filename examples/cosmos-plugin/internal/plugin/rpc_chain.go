package cosmos

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	rpcinternal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/rpc"
	"github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
)

func (n *CosmosNetwork) GetBlockHeight(ctx context.Context, rpcEndpoint string) (*plugin.BlockHeightResponse, error) {
	ctx = ensureContext(ctx)
	ctx, cancel := n.withRequestTimeout(ctx)
	defer cancel()

	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")
	if rpc == "" {
		return blockHeightError(missingEndpointError("RPC"))
	}

	var statusResp struct {
		Result struct {
			SyncInfo struct {
				LatestBlockHeight string `json:"latest_block_height"`
			} `json:"sync_info"`
		} `json:"result"`
	}
	if err := n.getJSON(ctx, rpc+"/status", &statusResp); err != nil {
		return blockHeightError(err)
	}

	height, err := strconv.ParseInt(statusResp.Result.SyncInfo.LatestBlockHeight, 10, 64)
	if err != nil {
		return blockHeightError(fmt.Errorf("failed to parse latest_block_height: %w", err))
	}

	return &plugin.BlockHeightResponse{Height: height}, nil
}

func (n *CosmosNetwork) GetBlockTime(ctx context.Context, rpcEndpoint string, sampleSize int) (*plugin.BlockTimeResponse, error) {
	ctx = ensureContext(ctx)

	if sampleSize < 2 {
		sampleSize = 10
	}

	heightResp, err := n.GetBlockHeight(ctx, rpcEndpoint)
	if err != nil {
		return blockTimeError(err)
	}
	if heightResp.Error != "" {
		return blockTimeError(errors.New(heightResp.Error))
	}

	current := heightResp.Height
	start := current - int64(sampleSize)
	if start < 1 {
		start = 1
	}

	t1, err := n.getBlockTimestamp(ctx, rpcEndpoint, start)
	if err != nil {
		return blockTimeError(err)
	}
	t2, err := n.getBlockTimestamp(ctx, rpcEndpoint, current)
	if err != nil {
		return blockTimeError(err)
	}

	count := current - start
	if count <= 0 {
		return blockTimeError(fmt.Errorf("insufficient block samples"))
	}
	avg := t2.Sub(t1) / time.Duration(count)
	if avg <= 0 {
		avg = defaultBlockTime
	}

	return &plugin.BlockTimeResponse{BlockTimeNs: int64(avg)}, nil
}

func (n *CosmosNetwork) IsChainRunning(ctx context.Context, rpcEndpoint string) (*plugin.ChainStatusResponse, error) {
	ctx = ensureContext(ctx)

	heightResp, err := n.GetBlockHeight(ctx, rpcEndpoint)
	if err != nil {
		return chainStatusError(err)
	}
	if heightResp.Error != "" {
		return chainStatusError(errors.New(heightResp.Error))
	}

	return &plugin.ChainStatusResponse{IsRunning: true}, nil
}

func (n *CosmosNetwork) WaitForBlock(ctx context.Context, rpcEndpoint string, targetHeight int64, timeoutMs int64) (*plugin.WaitForBlockResponse, error) {
	ctx = ensureContext(ctx)

	timeout := n.waitBlockTimeout
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(blockPollInterval)
	defer ticker.Stop()

	var lastHeight int64
	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if errors.Is(err, context.Canceled) {
				return waitForBlockError(lastHeight, fmt.Errorf("wait canceled"))
			}
			return waitForBlockError(lastHeight, fmt.Errorf("timeout waiting for height %d", targetHeight))
		case <-ticker.C:
			resp, err := n.GetBlockHeight(ctx, rpcEndpoint)
			if err != nil {
				continue
			}
			if resp == nil || resp.Error != "" {
				continue
			}
			lastHeight = resp.Height
			if resp.Height >= targetHeight {
				return &plugin.WaitForBlockResponse{CurrentHeight: resp.Height, Reached: true}, nil
			}
		}
	}
}

func (n *CosmosNetwork) GetAppVersion(ctx context.Context, rpcEndpoint string) (*plugin.AppVersionResponse, error) {
	ctx = ensureContext(ctx)
	ctx, cancel := n.withRequestTimeout(ctx)
	defer cancel()

	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")
	if rpc == "" {
		return appVersionError(missingEndpointError("RPC"))
	}

	var result struct {
		Result struct {
			Response struct {
				Version string `json:"version"`
			} `json:"response"`
		} `json:"result"`
	}
	if err := n.getJSON(ctx, rpc+"/abci_info", &result); err != nil {
		return appVersionError(err)
	}

	return &plugin.AppVersionResponse{Version: result.Result.Response.Version}, nil
}

func (n *CosmosNetwork) getBlockTimestamp(ctx context.Context, rpcEndpoint string, height int64) (time.Time, error) {
	rpc := n.resolveRPCEndpoint(rpcEndpoint, "")
	if rpc == "" {
		return time.Time{}, missingEndpointError("RPC")
	}

	var resp struct {
		Result struct {
			Block struct {
				Header struct {
					Time string `json:"time"`
				} `json:"header"`
			} `json:"block"`
		} `json:"result"`
	}
	if err := n.getJSON(ctx, fmt.Sprintf("%s/block?height=%d", rpc, height), &resp); err != nil {
		return time.Time{}, err
	}
	if resp.Result.Block.Header.Time == "" {
		return time.Time{}, fmt.Errorf("missing block time for height %d", height)
	}

	if t, err := time.Parse(time.RFC3339Nano, resp.Result.Block.Header.Time); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, resp.Result.Block.Header.Time); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("failed to parse block time %q", resp.Result.Block.Header.Time)
}

func (n *CosmosNetwork) withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := n.requestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return rpcinternal.WithTimeoutIfMissing(ctx, timeout)
}
