package main

import (
	cmdvalidation "github.com/altuslabsxyz/devnet-builder/internal/cmd/validation"
	"github.com/spf13/cobra"
)

const daemonRequiredAnnotation = "dvb.devnet-builder.io/requires-daemon"

func markDaemonRequired(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[daemonRequiredAnnotation] = "true"
	return cmd
}

func commandRequiresDaemon(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations != nil && c.Annotations[daemonRequiredAnnotation] == "true" {
			return true
		}
	}
	return false
}

func validateDaemonRequirement(cmd *cobra.Command) error {
	return cmdvalidation.RequireDaemonConnected(daemonClient != nil)
}
