package cosmos

import (
	"net/http"
	"time"
)

const (
	decimalPrecision18      = ".000000000000000000"
	defaultBlockTime        = 6 * time.Second
	defaultWaitTimeout      = 10 * time.Minute
	blockPollInterval       = 2 * time.Second
	snapshotResolverTimeout = 10 * time.Second
	defaultRequestTimeout   = 15 * time.Second

	listenHostAll                = "0.0.0.0"
	consensusTimeoutCommitDevnet = "1s"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}
