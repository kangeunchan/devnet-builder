package config

import (
	"fmt"

	"github.com/altuslabsxyz/devnet-builder/types"
)

const (
	// ValidatorCountMin is the minimum validators count for all modes.
	ValidatorCountMin = 1
	// ValidatorCountMaxLocal is the maximum validators count for local mode.
	ValidatorCountMaxLocal = 4
	// ValidatorCountMaxDocker is the maximum validators count for docker mode.
	ValidatorCountMaxDocker = 100
)

// ValidatorCountRangeForMode returns validator bounds for the given mode.
// Empty mode defaults to docker mode.
func ValidatorCountRangeForMode(mode string) (min int, max int, resolvedMode string, err error) {
	m := types.ExecutionMode(mode)
	if m == "" {
		m = types.ExecutionModeDocker
	}

	switch m {
	case types.ExecutionModeLocal:
		return ValidatorCountMin, ValidatorCountMaxLocal, string(types.ExecutionModeLocal), nil
	case types.ExecutionModeDocker:
		return ValidatorCountMin, ValidatorCountMaxDocker, string(types.ExecutionModeDocker), nil
	default:
		return 0, 0, "", fmt.Errorf("invalid mode: %s (must be 'docker' or 'local')", mode)
	}
}

// ValidateValidatorCount validates validators count using mode-aware constraints.
// Empty mode defaults to docker mode.
func ValidateValidatorCount(mode string, validators int) error {
	min, max, resolvedMode, err := ValidatorCountRangeForMode(mode)
	if err != nil {
		return err
	}

	if validators < min || validators > max {
		return fmt.Errorf("invalid validators: %d (must be %d-%d for %s mode)", validators, min, max, resolvedMode)
	}

	return nil
}
