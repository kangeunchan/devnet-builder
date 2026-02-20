package cosmos

import (
	"net/http"
	"time"
)

const (
	mainnetChainID = "cosmoshub-4"
	testnetChainID = "provider"

	mainnetRPC              = "https://cosmos-rpc.polkachu.com"
	mainnetREST             = "https://cosmos-api.polkachu.com"
	testnetRPC              = "https://cosmos-testnet-rpc.polkachu.com"
	testnetREST             = "https://cosmos-testnet-api.polkachu.com"
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
