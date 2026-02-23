package genesis

import (
	"fmt"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

func ValidateOptions(requireValidators bool, opts network.GenesisOptions) error {
	if requireValidators && len(opts.Validators) == 0 {
		return fmt.Errorf("genesis options validators are required for validator-set replacement")
	}
	return nil
}
