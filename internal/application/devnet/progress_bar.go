package devnet

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

func printByteProgress(logger ports.Logger, label string, current, total int64, startedAt time.Time) {
	if logger == nil {
		return
	}

	if current < 0 {
		current = 0
	}
	if total < 0 {
		total = 0
	}
	if total > 0 && current > total {
		total = current
	}

	const width = 30
	filled := 0
	percent := 0.0
	if total > 0 {
		percent = float64(current) / float64(total) * 100
		if percent > 100 {
			percent = 100
		}
		filled = int(float64(width) * percent / 100)
		if filled > width {
			filled = width
		}
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	currentMB := float64(current) / (1024 * 1024)
	totalMB := float64(total) / (1024 * 1024)

	elapsed := time.Since(startedAt).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	speedMB := currentMB / elapsed

	etaText := "--"
	if speedMB > 0 && total > 0 && current < total {
		remainingMB := totalMB - currentMB
		etaSeconds := remainingMB / speedMB
		switch {
		case etaSeconds < 60:
			etaText = fmt.Sprintf("%.0fs", etaSeconds)
		case etaSeconds < 3600:
			etaText = fmt.Sprintf("%.1fm", etaSeconds/60)
		default:
			etaText = fmt.Sprintf("%.1fh", etaSeconds/3600)
		}
	}

	if total > 0 {
		logger.Print("\r%s [%s] %5.1f%% | %.1f/%.1f MB | %.1f MB/s | ETA: %s",
			label, bar, percent, currentMB, totalMB, speedMB, etaText)
	} else {
		logger.Print("\r%s [%-30s] %.1f MB | %.1f MB/s",
			label, bar, currentMB, speedMB)
	}

	if total > 0 && current >= total {
		logger.Println("")
	}
}

func streamFileProgress(ctx context.Context, logger ports.Logger, label string, estimatedTotal int64, interval time.Duration, paths ...string) func() {
	if logger == nil {
		return func() {}
	}
	if len(paths) == 0 {
		return func() {}
	}

	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		startedAt := time.Now()
		lastShown := int64(-1)
		printed := false
		completed := false

		show := func(size int64) {
			total := estimatedTotal
			if total < size {
				total = size
			}
			printByteProgress(logger, label, size, total, startedAt)
			lastShown = size
			printed = true
			completed = total > 0 && size >= total
		}

		for {
			select {
			case <-ctx.Done():
				if size, ok := firstExistingFileSize(paths); ok {
					show(size)
				}
				if printed && !completed {
					logger.Println("")
				}
				return
			case <-ticker.C:
				size, ok := firstExistingFileSize(paths)
				if !ok {
					continue
				}
				if size == lastShown {
					continue
				}
				show(size)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func firstExistingFileSize(paths []string) (int64, bool) {
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		return st.Size(), true
	}
	return 0, false
}
