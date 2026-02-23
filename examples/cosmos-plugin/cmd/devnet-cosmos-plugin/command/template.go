package command

import (
	"fmt"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
	"github.com/spf13/cobra"
)

func NewPrintTemplateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "print-template",
		Short: "Print default customization template as YAML",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			yamlBytes, err := cosmos.EncodeCustomizationYAML(cosmos.DefaultCustomization())
			if err != nil {
				return fmt.Errorf("render customization template: %w", err)
			}
			_, err = cmd.OutOrStdout().Write(yamlBytes)
			return err
		},
	}
}
