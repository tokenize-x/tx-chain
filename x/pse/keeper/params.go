package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/samber/lo"

	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
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

	distributionID, err := k.getNextDistributionID(ctx)
	if err != nil {
		return err
	}

	// When addresses are removed from exclusion, move their accumulated ExcludedAddressScore
	// into AccountScoreSnapshot so they participate in future distributions with full earned score.
	// DelegationTimeEntries already exist (hooks tracked them while excluded), so no recreation needed.
	for _, addrStr := range addressesToRemove {
		addr, err := k.addressCodec.StringToBytes(addrStr)
		if err != nil {
			return err
		}
		if err := k.moveExcludedScoreToSnapshot(ctx, distributionID, addr); err != nil {
			return err
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

	// When addresses are added to exclusion, move their AccountScoreSnapshot into ExcludedAddressScore.
	// DelegationTimeEntries are kept (hooks will route scores to dedicated store).
	for _, addrStr := range toActuallyAdd {
		if err = k.moveSnapshotToExcludedScore(ctx, distributionID, addrStr); err != nil {
			return err
		}
	}

	return k.SetParams(ctx, params)
}

// moveExcludedScoreToSnapshot moves accumulated ExcludedAddressScore into AccountScoreSnapshot
// when an address is removed from the exclusion list. This preserves all earned score.
func (k Keeper) moveExcludedScoreToSnapshot(ctx context.Context, distributionID uint64, addr sdk.AccAddress) error {
	excludedScore, err := k.ExcludedAddressScore.Get(ctx, addr)
	if errors.Is(err, collections.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	if excludedScore.IsPositive() {
		if err := k.addToScore(ctx, distributionID, addr, excludedScore); err != nil {
			return err
		}
	}

	return k.ExcludedAddressScore.Remove(ctx, addr)
}

// moveSnapshotToExcludedScore moves AccountScoreSnapshot into ExcludedAddressScore
// when an address is added to the exclusion list. DelegationTimeEntries are kept intact.
func (k Keeper) moveSnapshotToExcludedScore(ctx context.Context, distributionID uint64, addrStr string) error {
	addr, err := k.addressCodec.StringToBytes(addrStr)
	if err != nil {
		return err
	}

	snapshotScore, err := k.GetDelegatorScore(ctx, distributionID, addr)
	if errors.Is(err, collections.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	if snapshotScore.IsPositive() {
		current, err := k.ExcludedAddressScore.Get(ctx, addr)
		if errors.Is(err, collections.ErrNotFound) {
			current = sdkmath.NewInt(0)
		} else if err != nil {
			return err
		}
		if err := k.ExcludedAddressScore.Set(ctx, addr, current.Add(snapshotScore)); err != nil {
			return err
		}
	}

	return k.RemoveDelegatorScore(ctx, distributionID, addr)
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

// IsExcludedAddress checks if the given address is in the excluded addresses list.
// Returns false if params are not initialized (e.g., during genesis).
func (k Keeper) IsExcludedAddress(ctx context.Context, addr sdk.AccAddress) (bool, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		// During genesis, params might not be initialized yet - treat all as non-excluded
		if errors.Is(err, collections.ErrNotFound) {
			return false, nil
		}
		// For other errors, return the error
		return false, err
	}

	addrStr, err := k.addressCodec.BytesToString(addr)
	if err != nil {
		return false, err
	}

	for _, excluded := range params.ExcludedAddresses {
		if excluded == addrStr {
			return true, nil
		}
	}
	return false, nil
}
