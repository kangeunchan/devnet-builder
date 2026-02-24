package commandcompat

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

var (
	longFlagPattern = regexp.MustCompile(`--[a-zA-Z0-9][a-zA-Z0-9-]*`)

	dockerStartHelpCache sync.Map
	localStartHelpCache  sync.Map

	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		return cmd.CombinedOutput()
	}
)

type startHelpCacheEntry struct {
	supported map[string]struct{}
	err       error
}

// FilterDockerStartArgs probes "start --help" for the selected docker image
// and removes unsupported long flags from the provided start arguments.
func FilterDockerStartArgs(
	ctx context.Context,
	image string,
	entrypoint string,
	args []string,
	logger ports.Logger,
) []string {
	image = strings.TrimSpace(image)
	if image == "" || len(args) == 0 {
		return args
	}

	key := fmt.Sprintf("docker|%s|%s", image, strings.TrimSpace(entrypoint))
	supported, err := getCachedSupportedStartFlags(&dockerStartHelpCache, key, func() (map[string]struct{}, error) {
		return probeDockerStartHelp(ctx, image, entrypoint)
	})
	if err != nil {
		if logger != nil {
			logger.Debug("Skipping docker start flag filtering: %v", err)
		}
		return args
	}

	filtered, removed := filterUnsupportedLongFlags(args, supported)
	if len(removed) > 0 && logger != nil {
		logger.Warn(
			"Removed unsupported start flag(s) for image %s: %s",
			image,
			strings.Join(removed, ", "),
		)
	}
	return filtered
}

// FilterLocalStartArgs probes "<binary> start --help" once per binary path
// and removes unsupported long flags from the provided start arguments.
func FilterLocalStartArgs(
	ctx context.Context,
	binaryPath string,
	args []string,
	logger ports.Logger,
) []string {
	binaryPath = strings.TrimSpace(binaryPath)
	if binaryPath == "" || len(args) == 0 {
		return args
	}

	key := fmt.Sprintf("local|%s", binaryPath)
	supported, err := getCachedSupportedStartFlags(&localStartHelpCache, key, func() (map[string]struct{}, error) {
		return probeLocalStartHelp(ctx, binaryPath)
	})
	if err != nil {
		if logger != nil {
			logger.Debug("Skipping local start flag filtering: %v", err)
		}
		return args
	}

	filtered, removed := filterUnsupportedLongFlags(args, supported)
	if len(removed) > 0 && logger != nil {
		logger.Warn(
			"Removed unsupported start flag(s) for binary %s: %s",
			binaryPath,
			strings.Join(removed, ", "),
		)
	}
	return filtered
}

func getCachedSupportedStartFlags(
	cache *sync.Map,
	key string,
	probe func() (map[string]struct{}, error),
) (map[string]struct{}, error) {
	if cached, ok := cache.Load(key); ok {
		entry := cached.(*startHelpCacheEntry)
		return entry.supported, entry.err
	}

	supported, err := probe()
	entry := &startHelpCacheEntry{
		supported: supported,
		err:       err,
	}

	actual, loaded := cache.LoadOrStore(key, entry)
	if loaded {
		existing := actual.(*startHelpCacheEntry)
		return existing.supported, existing.err
	}
	return entry.supported, entry.err
}

func probeDockerStartHelp(ctx context.Context, image, entrypoint string) (map[string]struct{}, error) {
	args := []string{"run", "--rm"}
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint != "" {
		args = append(args, "--entrypoint", entrypoint)
	}
	args = append(args, image, "start", "--help")

	output, err := runCommand(ctx, "docker", args...)
	if err != nil {
		return nil, fmt.Errorf(
			"docker start --help probe failed (image=%s, entrypoint=%s): %w (output=%s)",
			image,
			entrypoint,
			err,
			summarizeCommandOutput(output),
		)
	}

	supported := extractLongFlags(string(output))
	if len(supported) == 0 {
		return nil, fmt.Errorf(
			"docker start --help probe returned no flags (image=%s, entrypoint=%s)",
			image,
			entrypoint,
		)
	}
	return supported, nil
}

func probeLocalStartHelp(ctx context.Context, binaryPath string) (map[string]struct{}, error) {
	output, err := runCommand(ctx, binaryPath, "start", "--help")
	if err != nil {
		return nil, fmt.Errorf(
			"local start --help probe failed (binary=%s): %w (output=%s)",
			binaryPath,
			err,
			summarizeCommandOutput(output),
		)
	}

	supported := extractLongFlags(string(output))
	if len(supported) == 0 {
		return nil, fmt.Errorf("local start --help probe returned no flags (binary=%s)", binaryPath)
	}
	return supported, nil
}

func extractLongFlags(helpOutput string) map[string]struct{} {
	matches := longFlagPattern.FindAllString(helpOutput, -1)
	if len(matches) == 0 {
		return nil
	}

	flags := make(map[string]struct{}, len(matches))
	for _, raw := range matches {
		flags[raw] = struct{}{}
	}
	return flags
}

func filterUnsupportedLongFlags(args []string, supported map[string]struct{}) ([]string, []string) {
	if len(args) == 0 || len(supported) == 0 {
		return args, nil
	}

	filtered := make([]string, 0, len(args))
	removedSet := make(map[string]struct{})

	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if arg == "--" {
			filtered = append(filtered, args[idx:]...)
			break
		}

		flag, isLongFlag := parseLongFlag(arg)
		if !isLongFlag {
			filtered = append(filtered, arg)
			continue
		}

		if _, ok := supported[flag]; ok {
			filtered = append(filtered, arg)
			continue
		}

		removedSet[flag] = struct{}{}
		if !strings.Contains(arg, "=") && idx+1 < len(args) && shouldSkipFlagValue(args[idx+1]) {
			idx++
		}
	}

	if len(removedSet) == 0 {
		return args, nil
	}

	removed := make([]string, 0, len(removedSet))
	for flag := range removedSet {
		removed = append(removed, flag)
	}
	sort.Strings(removed)

	return filtered, removed
}

func parseLongFlag(arg string) (string, bool) {
	if !strings.HasPrefix(arg, "--") || arg == "--" {
		return "", false
	}

	if eq := strings.IndexByte(arg, '='); eq > 0 {
		return arg[:eq], true
	}
	return arg, true
}

func shouldSkipFlagValue(nextArg string) bool {
	nextArg = strings.TrimSpace(nextArg)
	if nextArg == "" || nextArg == "--" {
		return false
	}
	return !strings.HasPrefix(nextArg, "-")
}

func summarizeCommandOutput(output []byte) string {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return "<empty>"
	}
	const maxLen = 240
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}

// InferBinaryFromDockerImage derives a binary name from docker image reference.
// Example: ghcr.io/cosmos/gaia:v25.3.2 -> gaiad
func InferBinaryFromDockerImage(image string) string {
	ref := strings.TrimSpace(image)
	if ref == "" {
		return ""
	}

	if digestIdx := strings.Index(ref, "@"); digestIdx >= 0 {
		ref = ref[:digestIdx]
	}

	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		ref = ref[:lastColon]
	}

	name := ref
	if lastSlash >= 0 {
		name = ref[lastSlash+1:]
	}
	if name == "" {
		return ""
	}
	if strings.HasSuffix(name, "d") {
		return name
	}
	return name + "d"
}
