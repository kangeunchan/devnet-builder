package cosmos

import (
	"net/http"
	"time"
)

const (
	mainnetChainID = "cosmoshub-4"
	testnetChainID = "provider"

	mainnetRPC      = "https://cosmos-rpc.polkachu.com"
	mainnetREST     = "https://cosmos-api.polkachu.com"
	testnetRPC      = "https://cosmos-testnet-rpc.polkachu.com"
	testnetREST     = "https://cosmos-testnet-api.polkachu.com"
	mainnetSnapshot = "https://snapshots.cosmos.directory/cosmoshub-4/latest.tar.lz4"
	testnetSnapshot = "https://snapshots.kjnodes.com/cosmoshub-testnet/snapshot_latest.tar.lz4"

	decimalPrecision18 = ".000000000000000000"
	defaultBlockTime   = 6 * time.Second
	defaultWaitTimeout = 10 * time.Minute
	blockPollInterval  = 2 * time.Second

	listenHostAll                = "0.0.0.0"
	consensusTimeoutCommitDevnet = "1s"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}
