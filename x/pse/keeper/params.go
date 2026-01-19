package keeper

import (
	"context"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
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
		return errorsmod.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
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

	// When addresses are removed from exclusion, recreate their DelegationTimeEntries with current state
	// so they start accumulating score immediately without requiring a delegation change.
	currentBlockTime := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	for _, addrStr := range addressesToRemove {
		addr, err := k.addressCodec.StringToBytes(addrStr)
		if err != nil {
			return err
		}

		// Query all current delegations for this address
		delAddrBech32, err := k.addressCodec.BytesToString(addr)
		if err != nil {
			return err
		}

		delegationResponse, err := k.stakingKeeper.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
			DelegatorAddr: delAddrBech32,
		})
		if err != nil {
			return err
		}

		// Recreate DelegationTimeEntry for each active delegation
		for _, delegation := range delegationResponse.DelegationResponses {
			valAddr, err := k.valAddressCodec.StringToBytes(delegation.Delegation.ValidatorAddress)
			if err != nil {
				continue
			}

			// Set entry with current block time and current shares
			if err := k.SetDelegationTimeEntry(ctx, valAddr, addr, types.DelegationTimeEntry{
				LastChangedUnixSec: currentBlockTime,
				Shares:             delegation.Delegation.Shares,
			}); err != nil {
				return err
			}
		}
	}

	excludedAddrMap := make(map[string]struct{}, len(params.ExcludedAddresses))
	for _, addr := range params.ExcludedAddresses {
		excludedAddrMap[addr] = struct{}{}
	}
	toActuallyAdd := lo.Filter(addressesToAdd, func(addr string, _ int) bool {
		_, exists := excludedAddrMap[addr]
		return !exists
	})

	params.ExcludedAddresses = append(params.ExcludedAddresses, toActuallyAdd...)

	// Clear AccountScoreSnapshot AND DelegationTimeEntries for newly excluded addresses.
	// Removing DelegationTimeEntries ensures they start completely fresh when re-included.
	// Entries will be recreated naturally when hooks fire after re-inclusion.
	for _, addrStr := range toActuallyAdd {
		addr, err := k.addressCodec.StringToBytes(addrStr)
		if err != nil {
			return err
		}

		// Remove snapshot if it exists
		_ = k.AccountScoreSnapshot.Remove(ctx, addr)

		// Remove all delegation time entries for this address
		rng := collections.NewPrefixedPairRange[sdk.AccAddress, sdk.ValAddress](addr)
		iter, err := k.DelegationTimeEntries.Iterate(ctx, rng)
		if err != nil {
			return err
		}
		defer iter.Close()

		for ; iter.Valid(); iter.Next() {
			kv, err := iter.KeyValue()
			if err != nil {
				return err
			}
			if err := k.DelegationTimeEntries.Remove(ctx, kv.Key); err != nil {
				return err
			}
		}
	}

	return k.SetParams(ctx, params)
}

// UpdateClearingAccountMappings updates the recipient mappings in params via governance.
// The mappings must contain exactly all eligible (non-excluded) clearing accounts - no more, no less.
// Note: All validation is performed in MsgUpdateClearingAccountMappings.ValidateBasic()
// before the proposal is stored. This keeper method only handles authority check and state updates.
func (k Keeper) UpdateClearingAccountMappings(
	ctx context.Context,
	authority string,
	mappings []types.ClearingAccountMapping,
) error {
	// Check authority (requires state access to k.authority)
	if k.authority != authority {
		return errorsmod.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	// Get current params
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	// Update recipient mappings
	// All validation is already done in ValidateBasic to prevent invalid proposals from being stored
	params.ClearingAccountMappings = mappings

	return k.SetParams(ctx, params)
}
