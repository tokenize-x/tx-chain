package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// SetDelegationTimeEntry saves DelegationTimeEntry into storages.
func (k Keeper) SetDelegationTimeEntry(
	ctx context.Context,
	valAddr sdk.ValAddress,
	delAddr sdk.AccAddress,
	entry types.DelegationTimeEntry,
) error {
	key := collections.Join(delAddr, valAddr)
	return k.DelegationTimeEntries.Set(ctx, key, entry)
}

// GetDelegationTimeEntry retrieves DelegationTimeEntry from storages.
func (k Keeper) GetDelegationTimeEntry(
	ctx context.Context,
	valAddr sdk.ValAddress,
	delAddr sdk.AccAddress,
) (types.DelegationTimeEntry, error) {
	key := collections.Join(delAddr, valAddr)
	return k.DelegationTimeEntries.Get(ctx, key)
}

// RemoveDelegationTimeEntry removes DelegationTimeEntry from storages.
func (k Keeper) RemoveDelegationTimeEntry(
	ctx context.Context,
	valAddr sdk.ValAddress,
	delAddr sdk.AccAddress,
) error {
	key := collections.Join(delAddr, valAddr)
	return k.DelegationTimeEntries.Remove(ctx, key)
}

// CalculateDelegatorScore calculates the current total score for a delegator.
// This includes both the accumulated score snapshot (from previous periods)
// and the current period score calculated on-demand from active delegations.
// Formula: total_score = accumulated_score + current_period_score.
func (k Keeper) CalculateDelegatorScore(ctx context.Context, delAddr sdk.AccAddress) (sdkmath.Int, error) {
	// Start with the accumulated score from the snapshot (previous periods)
	accumulatedScore, err := k.AccountScoreSnapshot.Get(ctx, delAddr)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			accumulatedScore = sdkmath.NewInt(0)
		} else {
			return sdkmath.Int{}, err
		}
	}

	// Calculate current period score from delegations for this specific delegator
	// Use prefix query to efficiently get only this delegator's entries
	rng := collections.NewPrefixedPairRange[sdk.AccAddress, sdk.ValAddress](delAddr)
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
		valAddr := kv.Key.K2()
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
