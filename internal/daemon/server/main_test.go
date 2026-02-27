package server

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreCurrent(),
		// StreamProvisionLogs can outlive test cancellation when channel-close
		// paths are exercised in isolated goroutines.
		goleak.IgnoreTopFunction("github.com/altuslabsxyz/devnet-builder/internal/daemon/server.(*DevnetService).StreamProvisionLogs"),
	)
}
