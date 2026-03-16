package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

// SetDelegationTimeEntry saves DelegationTimeEntry into storages.
func (k Keeper) SetDelegationTimeEntry(
	ctx context.Context,
	distributionID uint64,
	valAddr sdk.ValAddress,
	delAddr sdk.AccAddress,
	entry types.DelegationTimeEntry,
) error {
	key := collections.Join3(distributionID, delAddr, valAddr)
	return k.DelegationTimeEntries.Set(ctx, key, entry)
}

// GetDelegationTimeEntry retrieves DelegationTimeEntry from storages.
func (k Keeper) GetDelegationTimeEntry(
	ctx context.Context,
	distributionID uint64,
	valAddr sdk.ValAddress,
	delAddr sdk.AccAddress,
) (types.DelegationTimeEntry, error) {
	key := collections.Join3(distributionID, delAddr, valAddr)
	return k.DelegationTimeEntries.Get(ctx, key)
}

// RemoveDelegationTimeEntry removes DelegationTimeEntry from storages.
func (k Keeper) RemoveDelegationTimeEntry(
	ctx context.Context,
	distributionID uint64,
	valAddr sdk.ValAddress,
	delAddr sdk.AccAddress,
) error {
	key := collections.Join3(distributionID, delAddr, valAddr)
	return k.DelegationTimeEntries.Remove(ctx, key)
}

// GetDelegatorScore gets the score for a delegator.
func (k Keeper) GetDelegatorScore(
	ctx context.Context,
	distributionID uint64,
	delAddr sdk.AccAddress,
) (sdkmath.Int, error) {
	key := collections.Join(distributionID, delAddr)
	return k.AccountScoreSnapshot.Get(ctx, key)
}

// SetDelegatorScore sets the score for a delegator.
func (k Keeper) SetDelegatorScore(
	ctx context.Context, distributionID uint64, delAddr sdk.AccAddress, score sdkmath.Int,
) error {
	key := collections.Join(distributionID, delAddr)
	return k.AccountScoreSnapshot.Set(ctx, key, score)
}

// RemoveDelegatorScore removes the score for a delegator.
func (k Keeper) RemoveDelegatorScore(ctx context.Context, distributionID uint64, delAddr sdk.AccAddress) error {
	key := collections.Join(distributionID, delAddr)
	return k.AccountScoreSnapshot.Remove(ctx, key)
}

// addToScore atomically adds a score value to a delegator's score snapshot
// and incrementally updates TotalScore for the same distribution.
func (k Keeper) addToScore(
	ctx context.Context, distributionID uint64, delAddr sdk.AccAddress, score sdkmath.Int,
) error {
	if score.IsZero() {
		return nil
	}
	lastScore, err := k.GetDelegatorScore(ctx, distributionID, delAddr)
	if errors.Is(err, collections.ErrNotFound) {
		lastScore = sdkmath.NewInt(0)
	} else if err != nil {
		return err
	}
	if err := k.SetDelegatorScore(ctx, distributionID, delAddr, lastScore.Add(score)); err != nil {
		return err
	}

	// Accumulate TotalScore
	currentTotal, err := k.TotalScore.Get(ctx, distributionID)
	if errors.Is(err, collections.ErrNotFound) {
		currentTotal = sdkmath.NewInt(0)
	} else if err != nil {
		return err
	}
	return k.TotalScore.Set(ctx, distributionID, currentTotal.Add(score))
}

// CalculateDelegatorScore calculates the current total score for a delegator.
// This includes both the accumulated score snapshot (from previous periods)
// and the current period score calculated on-demand from active delegations.
// Formula: total_score = accumulated_score + current_period_score.
func (k Keeper) CalculateDelegatorScore(ctx context.Context, delAddr sdk.AccAddress) (sdkmath.Int, error) {
	// Find the distribution ID where current scores are stored.
	distributionID, err := k.getActiveDistributionID(ctx)
	if err != nil {
		return sdkmath.Int{}, err
	}

	// Start with the accumulated score from the snapshot (previous periods)
	accumulatedScore, err := k.GetDelegatorScore(ctx, distributionID, delAddr)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			accumulatedScore = sdkmath.NewInt(0)
		} else {
			return sdkmath.Int{}, err
		}
	}

	// Calculate current period score from delegations for this specific delegator
	// Use prefix query to efficiently get only this delegator's entries
	rng := collections.NewSuperPrefixedTripleRange[uint64, sdk.AccAddress, sdk.ValAddress](distributionID, delAddr)
	iter, err := k.DelegationTimeEntries.Iterate(ctx, rng)
	if err != nil {
		return sdkmath.Int{}, err
	}
	defer iter.Close()

	currentPeriodScore := sdkmath.NewInt(0)
	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return sdkmath.Int{}, err
		}

		// Now we only iterate entries for this specific delegator
		valAddr := kv.Key.K3()
		delegationTimeEntry := kv.Value
		addedScore, err := calculateAddedScore(ctx, k, valAddr, delegationTimeEntry)
		if err != nil {
			return sdkmath.Int{}, err
		}

		currentPeriodScore = currentPeriodScore.Add(addedScore)
	}

	// Return total score = accumulated + current period
	totalScore := accumulatedScore.Add(currentPeriodScore)
	return totalScore, nil
}
