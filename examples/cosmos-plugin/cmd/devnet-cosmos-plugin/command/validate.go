package command

import (
	"fmt"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
	"github.com/spf13/cobra"
)

func NewValidateCmd(networkModule network.Module) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate plugin configuration and exit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := networkModule.Validate(); err != nil {
				return fmt.Errorf("module validation failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	}
}
