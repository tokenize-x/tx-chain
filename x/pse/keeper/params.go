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
func (k Keeper) UpdateClearingMappings(
	ctx context.Context,
	authority string,
	mappings []types.ClearingAccountMapping,
) error {
	if k.authority != authority {
		return errors.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	// Get current params
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	// Build a set of clearing accounts that are currently in the allocation schedule
	requiredAccounts := make(map[string]bool)
	iter, err := k.AllocationSchedule.Iterate(ctx, nil)
	if err != nil {
		return err
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return err
		}
		for _, alloc := range kv.Value.Allocations {
			requiredAccounts[alloc.ClearingAccount] = true
		}
	}

	// Build a set of module accounts in the new mappings
	newMappings := make(map[string]bool)
	for _, mapping := range mappings {
		newMappings[mapping.ClearingAccount] = true
	}

	// Check that all required clearing accounts are present in the new mappings
	// Excluded clearing accounts (like Community) don't need mappings since they don't distribute to recipients
	for clearingAccount := range requiredAccounts {
		// Skip excluded clearing accounts - they don't need recipient mappings
		if types.IsExcludedClearingAccount(clearingAccount) {
			continue
		}
		if !newMappings[clearingAccount] {
			return errors.Wrapf(types.ErrInvalidInput,
				"cannot remove mapping for clearing account '%s': it is still referenced in the allocation schedule", clearingAccount)
		}
	}

	// Update sub account mappings
	params.ClearingAccountMappings = mappings

	return k.SetParams(ctx, params)
}
