package cosmos

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type commandRunnerFunc func(context.Context, string, ...string) ([]byte, error)

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx = ensureContext(ctx)

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("command failed: %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return out, nil
}

func (n *CosmosNetwork) commandRunner() commandRunnerFunc {
	if n != nil && n.runCmd != nil {
		return n.runCmd
	}
	return runCommand
}
