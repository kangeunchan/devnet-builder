package validation

import (
	"errors"
	"fmt"

	"github.com/altuslabsxyz/devnet-builder/types"
)

var (
	// ErrDaemonNotRunning is returned when a command requires daemon connectivity.
	ErrDaemonNotRunning = errors.New("daemon not running - start with: devnetd")
)

// ValidateMode validates execution mode values accepted by CLI commands.
func ValidateMode(mode string) error {
	if !types.ExecutionMode(mode).IsValid() {
		return fmt.Errorf("invalid mode: %s (must be 'docker' or 'local')", mode)
	}
	return nil
}

// ValidateNetworkSource validates snapshot source network values.
func ValidateNetworkSource(network string) error {
	if !types.NetworkSource(network).IsValid() {
		return fmt.Errorf("invalid network: %s (must be 'mainnet' or 'testnet')", network)
	}
	return nil
}

// ValidateValidatorsForMode validates validator count with mode-specific bounds.
func ValidateValidatorsForMode(mode string, validators int) error {
	switch types.ExecutionMode(mode) {
	case types.ExecutionModeDocker:
		if validators < 1 || validators > 100 {
			return fmt.Errorf("invalid validators: %d (must be 1-100 for docker mode)", validators)
		}
	case types.ExecutionModeLocal:
		if validators < 1 || validators > 4 {
			return fmt.Errorf("invalid validators: %d (must be 1-4 for local mode)", validators)
		}
	default:
		return ValidateMode(mode)
	}

	return nil
}

// ValidateValidatorsRange validates validator count against a fixed range.
func ValidateValidatorsRange(validators, min, max int) error {
	if validators < min || validators > max {
		return fmt.Errorf("invalid validators: %d (must be %d-%d)", validators, min, max)
	}
	return nil
}

// ValidateAtLeast validates a numeric value has a minimum.
func ValidateAtLeast(flagName string, value, min int) error {
	if value < min {
		return fmt.Errorf("--%s must be at least %d", flagName, min)
	}
	return nil
}

// ValidateNonNegative validates a numeric value is non-negative.
func ValidateNonNegative(flagName string, value int) error {
	if value < 0 {
		return fmt.Errorf("--%s cannot be negative", flagName)
	}
	return nil
}

// RequireDaemonConnected validates daemon connectivity for daemon-dependent commands.
func RequireDaemonConnected(connected bool) error {
	if !connected {
		return ErrDaemonNotRunning
	}
	return nil
}
