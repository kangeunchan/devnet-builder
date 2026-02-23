package cosmos

import (
	"net/http"
	"time"
)

const (
	decimalPrecision18             = ".000000000000000000"
	defaultBlockTime               = 6 * time.Second
	defaultWaitTimeout             = 10 * time.Minute
	blockPollInterval              = 2 * time.Second
	defaultSnapshotResolverTimeout = 10 * time.Second
	defaultRequestTimeout          = 15 * time.Second
)

// httpClient relies on request context deadlines for timeout control.
var httpClient = &http.Client{}
