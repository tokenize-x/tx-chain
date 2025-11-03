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
