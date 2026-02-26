package main

import (
	"fmt"
	"os"

	"github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/cmd/devnet-cosmos-plugin/command"
	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
	"github.com/altuslabsxyz/devnet-builder/internal/version"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
	"github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
	"github.com/spf13/cobra"
)

const (
	appName    = "devnet-cosmos-plugin"
	serverName = "devnet-builder"
)

func main() {
	rootCmd := newRootCmd(cosmos.New())
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd(networkModule network.Module) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   appName,
		Short: "Cosmos Hub plugin for devnet-builder",
		Long:  "Plugin binary that serves the Cosmos Hub network module for devnet-builder.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(networkModule)
		},
	}

	rootCmd.AddCommand(version.NewCmd(appName, serverName))
	rootCmd.AddCommand(command.NewValidateCmd(networkModule))

	return rootCmd
}

func runServe(networkModule network.Module) error {
	if err := networkModule.Validate(); err != nil {
		return fmt.Errorf("module validation failed: %w", err)
	}

	plugin.Serve(networkModule)
	return nil
}
