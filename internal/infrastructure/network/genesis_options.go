package network

import (
	"fmt"
	"strings"

	pkgNetwork "github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// ToPkgGenesisOptions converts internal genesis options to pkg/network options.
func ToPkgGenesisOptions(opts GenesisOptions) (pkgNetwork.GenesisOptions, error) {
	validators := make([]pkgNetwork.ValidatorInfo, len(opts.Validators))
	for i, validator := range opts.Validators {
		if strings.TrimSpace(validator.OperatorAddress) == "" {
			return pkgNetwork.GenesisOptions{}, fmt.Errorf("invalid genesis validator %d: operator_address is required", i)
		}
		validators[i] = pkgNetwork.ValidatorInfo{
			Moniker:         validator.Moniker,
			ConsPubKey:      validator.ConsPubKey,
			OperatorAddress: validator.OperatorAddress,
			SelfDelegation:  validator.SelfDelegation,
		}
	}

	accounts := make([]pkgNetwork.GenesisAccountInfo, len(opts.Accounts))
	for i, account := range opts.Accounts {
		if strings.TrimSpace(account.Address) == "" {
			return pkgNetwork.GenesisOptions{}, fmt.Errorf("invalid genesis account %d: address is required", i)
		}

		balance, err := NormalizeGenesisAccountBalance(account.Balance, account.Coins)
		if err != nil {
			return pkgNetwork.GenesisOptions{}, fmt.Errorf("invalid genesis account %d (%s): %w", i, account.Address, err)
		}
		accounts[i] = pkgNetwork.GenesisAccountInfo{
			Name:    account.Name,
			Address: account.Address,
			Balance: balance,
		}
	}

	return pkgNetwork.GenesisOptions{
		ChainID:       opts.ChainID,
		NumValidators: opts.NumValidators,
		Validators:    validators,
		AddAccounts:   accounts,
	}, nil
}
