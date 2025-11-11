package keeper

import (
	"context"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// GetParams returns the current pse module parameters.
func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return types.Params{}, err
	}
	return params, nil
}

// SetParams sets the pse module parameters.
func (k Keeper) SetParams(ctx context.Context, params types.Params) error {
	if err := params.ValidateBasic(); err != nil {
		return err
	}
	return k.Params.Set(ctx, params)
}

// UpdateExcludedAddresses updates the excluded addresses list in params via governance.
func (k Keeper) UpdateExcludedAddresses(
	ctx context.Context,
	authority string,
	addressesToAdd, addressesToRemove []string,
) error {
	if k.authority != authority {
		return errors.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	// Get current params
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	addressesToRemoveSet := make(map[string]struct{}, len(addressesToRemove))
	for _, addr := range addressesToRemove {
		addressesToRemoveSet[addr] = struct{}{}
	}
	params.ExcludedAddresses = lo.Filter(params.ExcludedAddresses, func(addr string, _ int) bool {
		_, found := addressesToRemoveSet[addr]
		return !found
	})

	excludedAddrMap := make(map[string]struct{}, len(params.ExcludedAddresses))
	for _, addr := range params.ExcludedAddresses {
		excludedAddrMap[addr] = struct{}{}
	}
	toActuallyAdd := lo.Filter(addressesToAdd, func(addr string, _ int) bool {
		_, exists := excludedAddrMap[addr]
		return !exists
	})

	params.ExcludedAddresses = append(params.ExcludedAddresses, toActuallyAdd...)

	return k.SetParams(ctx, params)
}

// UpdateClearingMappings updates the sub account mappings in params via governance.
// The mappings must contain exactly all eligible (non-excluded) clearing accounts - no more, no less.
func (k Keeper) UpdateClearingMappings(
	ctx context.Context,
	authority string,
	mappings []types.ClearingAccountMapping,
) error {
	if k.authority != authority {
		return errors.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	// Get all eligible (non-excluded) clearing accounts that must be present
	eligibleAccounts := types.GetNonCommunityClearingAccounts()

	// Build a set of clearing accounts in the new mappings
	mappingAccounts := make(map[string]bool)
	for _, mapping := range mappings {
		mappingAccounts[mapping.ClearingAccount] = true
	}

	// Check for missing eligible accounts
	var missingAccounts []string
	for _, eligibleAccount := range eligibleAccounts {
		if !mappingAccounts[eligibleAccount] {
			missingAccounts = append(missingAccounts, eligibleAccount)
		}
	}

	if len(missingAccounts) > 0 {
		return errors.Wrapf(types.ErrInvalidInput,
			"there are missing non-Community clearing accounts in the mappings")
	}

	// Check for extra accounts (accounts that are not eligible)
	// Build a set of eligible accounts for quick lookup
	eligibleSet := make(map[string]bool)
	for _, eligibleAccount := range eligibleAccounts {
		eligibleSet[eligibleAccount] = true
	}

	var extraAccounts []string
	for _, mapping := range mappings {
		// Check if it's not in the eligible list
		if !eligibleSet[mapping.ClearingAccount] {
			extraAccounts = append(extraAccounts, mapping.ClearingAccount)
		}
	}

	if len(extraAccounts) > 0 {
		return errors.Wrapf(types.ErrInvalidInput,
			"mappings contain invalid non-Community clearing accounts")
	}

	// Verify that the number of mappings matches the number of eligible accounts
	if len(mappings) != len(eligibleAccounts) {
		return errors.Wrapf(types.ErrInvalidInput,
			"expected exact number of mappings (one for each non-Community clearing account)")
	}

	// Get current params
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	// Update sub account mappings
	params.ClearingAccountMappings = mappings

	return k.SetParams(ctx, params)
}
