package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultParams returns default pse module parameters.
func DefaultParams() Params {
	return Params{
		ExcludedAddresses: []string{},
	}
}

// ValidateBasic performs basic validation on pse module parameters.
func (p Params) ValidateBasic() error {
	// Validate excluded addresses
	return validateExcludedAddresses(p.ExcludedAddresses)
}

func validateExcludedAddresses(addresses []string) error {
	seen := make(map[string]bool)

	for i, addr := range addresses {
		// Validate address format
		if _, err := sdk.AccAddressFromBech32(addr); err != nil {
			return fmt.Errorf("excluded address %d: invalid address %s: %w", i, addr, err)
		}

		// Check for duplicates
		if seen[addr] {
			return fmt.Errorf("excluded address %d: duplicate address %s", i, addr)
		}
		seen[addr] = true
	}

	return nil
}
