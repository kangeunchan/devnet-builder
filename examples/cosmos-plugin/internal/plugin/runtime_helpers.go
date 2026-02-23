package cosmos

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	envGenesisBalanceWorkers = "DEVNET_GENESIS_BALANCE_WORKERS"
	maxGenesisBalanceWorkers = 32
)

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func parseDurationWithFallback(raw string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fallback
	}

	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}

	return d
}

func genesisBalanceWorkers() int {
	defaultWorkers := runtime.GOMAXPROCS(0)
	if defaultWorkers < 1 {
		defaultWorkers = 1
	}
	if defaultWorkers > maxGenesisBalanceWorkers {
		defaultWorkers = maxGenesisBalanceWorkers
	}

	raw := strings.TrimSpace(os.Getenv(envGenesisBalanceWorkers))
	if raw == "" {
		return defaultWorkers
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return defaultWorkers
	}
	if parsed > maxGenesisBalanceWorkers {
		return maxGenesisBalanceWorkers
	}
	return parsed
}
