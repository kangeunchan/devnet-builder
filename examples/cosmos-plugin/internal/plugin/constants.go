package cosmos

import (
	"net/http"
	"time"
)

const (
	mainnetChainID = "cosmoshub-4"
	testnetChainID = "provider"

	mainnetRPC  = "https://cosmoshub.rpc.kjnodes.com"
	mainnetREST = "https://cosmoshub.api.kjnodes.com"
	testnetRPC  = "https://cosmoshub-testnet.rpc.kjnodes.com"
	testnetREST = "https://cosmoshub-testnet.api.kjnodes.com"
	mainnetSnapshotIndexURL = "https://www.polkachu.com/tendermint_snapshots/cosmos"
	testnetSnapshotIndexURL = "https://www.polkachu.com/testnets/cosmos/snapshots"

	decimalPrecision18      = ".000000000000000000"
	defaultBlockTime        = 6 * time.Second
	defaultWaitTimeout      = 10 * time.Minute
	blockPollInterval       = 2 * time.Second
	snapshotResolverTimeout = 10 * time.Second

	listenHostAll                = "0.0.0.0"
	consensusTimeoutCommitDevnet = "1s"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}
