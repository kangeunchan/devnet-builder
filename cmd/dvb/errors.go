// cmd/dvb/errors.go
package main

import cmdvalidation "github.com/altuslabsxyz/devnet-builder/internal/cmd/validation"

// errDaemonNotRunning is the standard error returned when daemon connection is required but unavailable.
var errDaemonNotRunning = cmdvalidation.ErrDaemonNotRunning

// requireDaemon returns errDaemonNotRunning if the daemon client is not connected.
// Usage: if err := requireDaemon(); err != nil { return err }
func requireDaemon() error {
	return cmdvalidation.RequireDaemonConnected(daemonClient != nil)
}
