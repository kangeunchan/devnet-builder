package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMarkDaemonRequiredAndLookup(t *testing.T) {
	parent := markDaemonRequired(&cobra.Command{Use: "parent"})
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)

	if !commandRequiresDaemon(child) {
		t.Fatalf("expected child command to inherit daemon requirement")
	}
}

func TestValidateDaemonRequirement(t *testing.T) {
	original := daemonClient
	daemonClient = nil
	t.Cleanup(func() {
		daemonClient = original
	})

	err := validateDaemonRequirement(&cobra.Command{Use: "any"})
	if err == nil {
		t.Fatalf("expected daemon requirement error")
	}
	if !strings.Contains(err.Error(), "daemon not running") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPreRunProvisionValidation(t *testing.T) {
	opts := &provisionOptions{
		quick:      true,
		network:    "stable",
		validators: 1,
		fullNodes:  0,
		mode:       "invalid",
	}

	err := preRunProvision(opts)(&cobra.Command{Use: "provision"}, nil)
	if err == nil {
		t.Fatalf("expected invalid mode error")
	}
	if !strings.Contains(err.Error(), "invalid mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}
