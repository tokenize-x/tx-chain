package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// Hooks implements the staking hooks interface.
type Hooks struct {
	k Keeper
}

var _ stakingtypes.StakingHooks = Hooks{}

// Hooks Create new distribution hooks.
func (k Keeper) Hooks() Hooks {
	return Hooks{k}
}

// AfterDelegationModified implements the staking hooks interface.
func (h Hooks) AfterDelegationModified(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	delegation, err := h.k.stakingKeeper.GetDelegation(ctx, delAddr, valAddr)
	if err != nil {
		return err
	}

	val, err := h.k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return err
	}

	blockTimeUnixSeconds := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	delegationTimeEntry, err := h.k.GetDelegationTimeEntry(ctx, valAddr, delAddr)
	if errors.Is(err, collections.ErrNotFound) {
		delegationTimeEntry = types.DelegationTimeEntry{
			LastChangedUnixSec: blockTimeUnixSeconds,
			Shares:             delegation.Shares,
		}
	} else if err != nil {
		return err
	}

	oldScore, err := h.k.AccountScore.Get(ctx, delAddr)
	if errors.Is(err, collections.ErrNotFound) {
		oldScore = sdkmath.NewInt(0)
	} else if err != nil {
		return err
	}

	oldDelegatedTokens := val.TokensFromShares(delegationTimeEntry.Shares).TruncateInt()
	delegationDuration := blockTimeUnixSeconds - delegationTimeEntry.LastChangedUnixSec
	addedScore := oldDelegatedTokens.MulRaw(delegationDuration)
	newScore := oldScore.Add(addedScore)

	if err := h.k.SetDelegationTimeEntry(ctx, valAddr, delAddr, types.DelegationTimeEntry{
		LastChangedUnixSec: blockTimeUnixSeconds,
		Shares:             delegation.Shares,
	}); err != nil {
		return err
	}

	return h.k.AccountScore.Set(ctx, delAddr, newScore)
}

// BeforeDelegationRemoved implements the staking hooks interface.
func (h Hooks) BeforeDelegationRemoved(_ context.Context, _ sdk.AccAddress, _ sdk.ValAddress) error {
	return nil
}

// The following hooks don't need to be implemented.

// AfterValidatorCreated implements the staking hooks interface.
func (h Hooks) AfterValidatorCreated(ctx context.Context, valAddr sdk.ValAddress) error {
	return nil
}

// AfterValidatorRemoved implements the staking hooks interface.
func (h Hooks) AfterValidatorRemoved(ctx context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// BeforeDelegationCreated implements the staking hooks interface.
func (h Hooks) BeforeDelegationCreated(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	return nil
}

// BeforeDelegationSharesModified implements the staking hooks interface.
func (h Hooks) BeforeDelegationSharesModified(
	ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress,
) error {
	return nil
}

// BeforeValidatorSlashed implements the staking hooks interface.
func (h Hooks) BeforeValidatorSlashed(ctx context.Context, valAddr sdk.ValAddress, fraction sdkmath.LegacyDec) error {
	return nil
}

// BeforeValidatorModified implements the staking hooks interface.
func (h Hooks) BeforeValidatorModified(_ context.Context, _ sdk.ValAddress) error {
	return nil
}

// AfterValidatorBonded implements the staking hooks interface.
func (h Hooks) AfterValidatorBonded(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// AfterValidatorBeginUnbonding implements the staking hooks interface.
func (h Hooks) AfterValidatorBeginUnbonding(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// AfterUnbondingInitiated implements the staking hooks interface.
func (h Hooks) AfterUnbondingInitiated(_ context.Context, _ uint64) error {
	return nil
}
