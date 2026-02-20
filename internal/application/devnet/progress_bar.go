package devnet

import (
	"strings"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

type spinnerStopper interface {
	StopSpinner()
}

func stopSpinnerIfSupported(logger ports.Logger) {
	if stopper, ok := logger.(spinnerStopper); ok {
		stopper.StopSpinner()
	}
}

func printLoopProgress(logger ports.Logger, label string, current, total int) {
	if logger == nil || total <= 0 {
		return
	}

	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}

	const width = 24
	filled := current * width / total
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
	logger.Print("\r%s [%s] %d/%d", label, bar, current, total)
	if current == total {
		logger.Println("")
	}
}
