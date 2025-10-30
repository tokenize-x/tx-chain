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

// Hooks Create new staking hooks.
func (k Keeper) Hooks() Hooks {
	return Hooks{k}
}

// AfterDelegationModified implements the staking hooks interface.
func (h Hooks) AfterDelegationModified(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	delegation, err := h.k.stakingKeeper.GetDelegation(ctx, delAddr, valAddr)
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

	lastScore, err := h.k.AccountScoreSnapshot.Get(ctx, delAddr)
	if errors.Is(err, collections.ErrNotFound) {
		lastScore = sdkmath.NewInt(0)
	} else if err != nil {
		return err
	}

	delegationScore, err := calculateAddedScore(ctx, h.k, valAddr, delegationTimeEntry)
	if err != nil {
		return err
	}
	newScore := lastScore.Add(delegationScore)

	if err := h.k.SetDelegationTimeEntry(ctx, valAddr, delAddr, types.DelegationTimeEntry{
		LastChangedUnixSec: blockTimeUnixSeconds,
		Shares:             delegation.Shares,
	}); err != nil {
		return err
	}

	return h.k.AccountScoreSnapshot.Set(ctx, delAddr, newScore)
}

// BeforeDelegationRemoved implements the staking hooks interface.
func (h Hooks) BeforeDelegationRemoved(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	delegationTimeEntry, err := h.k.GetDelegationTimeEntry(ctx, valAddr, delAddr)
	if err != nil {
		return err
	}

	lastScore, err := h.k.AccountScoreSnapshot.Get(ctx, delAddr)
	if errors.Is(err, collections.ErrNotFound) {
		lastScore = sdkmath.NewInt(0)
	} else if err != nil {
		return err
	}

	addedScore, err := calculateAddedScore(ctx, h.k, valAddr, delegationTimeEntry)
	if err != nil {
		return err
	}
	newScore := lastScore.Add(addedScore)

	if err := h.k.RemoveDelegationTimeEntry(ctx, valAddr, delAddr); err != nil {
		return err
	}

	return h.k.AccountScoreSnapshot.Set(ctx, delAddr, newScore)
}

func calculateAddedScore(
	ctx context.Context,
	keeper Keeper,
	valAddr sdk.ValAddress,
	delegationTimeEntry types.DelegationTimeEntry,
) (sdkmath.Int, error) {
	val, err := keeper.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return sdkmath.NewInt(0), err
	}

	blockTimeUnixSeconds := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	delegationDuration := blockTimeUnixSeconds - delegationTimeEntry.LastChangedUnixSec
	previousDelegatedTokens := val.TokensFromShares(delegationTimeEntry.Shares).TruncateInt()
	delegationScore := previousDelegatedTokens.MulRaw(delegationDuration)
	return delegationScore, nil
}

// BeforeValidatorSlashed implements the staking hooks interface.
// TODO: we need to handle validator slashing for a more accurate score calculation.
// example:
// lets assume a validator is slashed at the middle of a given period which splits the period into two parts,
// given following:
// before slashing: 10 ucore
// after slashing: 9 ucore
// we calculate score as 9ucore * period (not completely accurate)
// more accurate formula should be 10ucore * period_1 + 9ucore * period_2.
func (h Hooks) BeforeValidatorSlashed(ctx context.Context, valAddr sdk.ValAddress, fraction sdkmath.LegacyDec) error {
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
