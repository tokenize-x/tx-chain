package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

// defaultBatchSize is the number of entries processed per EndBlock during multi-block distribution.
const defaultBatchSize = 100 // TODO: make configurable

// ProcessPhase1ScoreConversion processes a batch of DelegationTimeEntries from the ongoing distribution (ongoingID),
// converting each entry into a score snapshot and migrating it to nextID (ongoingID + 1).
//
// For each entry in the batch:
//  1. Calculate score from lastChanged to distribution timestamp -> addToScore(ongoingID)
//  2. Create new entry under nextID with same shares, lastChanged = distTimestamp
//  3. Remove entry from ongoingID
//
// Returns true when all ongoingID entries have been processed and TotalScore is computed.
func (k Keeper) ProcessPhase1ScoreConversion(ctx context.Context, ongoing types.ScheduledDistribution) (bool, error) {
	ongoingID := ongoing.ID
	nextID := ongoing.ID + 1
	distTimestamp := int64(ongoing.Timestamp)

	// Collect a batch of entries from ongoingID.
	iter, err := k.DelegationTimeEntries.Iterate(
		ctx,
		collections.NewPrefixedTripleRange[uint64, sdk.AccAddress, sdk.ValAddress](ongoingID),
	)
	if err != nil {
		return false, err
	}

	type entryKV struct {
		delAddr sdk.AccAddress
		valAddr sdk.ValAddress
		entry   types.DelegationTimeEntry
	}

	batch := make([]entryKV, 0, defaultBatchSize)
	for ; iter.Valid() && len(batch) < defaultBatchSize; iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			iter.Close()
			return false, err
		}
		batch = append(batch, entryKV{
			delAddr: kv.Key.K2(),
			valAddr: kv.Key.K3(),
			entry:   kv.Value,
		})
	}
	iter.Close()

	// Compute TotalScore from all accumulated snapshots.
	if len(batch) == 0 {
		if err := k.computeTotalScore(ctx, ongoingID); err != nil {
			return false, err
		}
		return true, nil
	}

	for _, item := range batch {
		isExcluded, err := k.IsExcludedAddress(ctx, item.delAddr)
		if err != nil {
			return false, err
		}

		if !isExcluded {
			score, err := calculateScoreAtTimestamp(ctx, k, item.valAddr, item.entry, distTimestamp)
			if err != nil {
				return false, err
			}
			if err := k.addToScore(ctx, ongoingID, item.delAddr, score); err != nil {
				return false, err
			}
		}

		// Migrate entry to nextID with same shares, reset lastChanged to distribution timestamp.
		if err := k.SetDelegationTimeEntry(ctx, nextID, item.valAddr, item.delAddr, types.DelegationTimeEntry{
			LastChangedUnixSec: distTimestamp,
			Shares:             item.entry.Shares,
		}); err != nil {
			return false, err
		}

		// Remove from ongoingID.
		if err := k.RemoveDelegationTimeEntry(ctx, ongoingID, item.valAddr, item.delAddr); err != nil {
			return false, err
		}
	}

	return false, nil
}

// computeTotalScore sums all AccountScoreSnapshot entries for a distribution and stores the result in TotalScore.
func (k Keeper) computeTotalScore(ctx context.Context, distributionID uint64) error {
	iter, err := k.AccountScoreSnapshot.Iterate(
		ctx,
		collections.NewPrefixedPairRange[uint64, sdk.AccAddress](distributionID),
	)
	if err != nil {
		return err
	}
	defer iter.Close()

	totalScore := sdkmath.NewInt(0)
	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return err
		}
		totalScore = totalScore.Add(kv.Value)
	}

	return k.TotalScore.Set(ctx, distributionID, totalScore)
}

// ProcessPhase2TokenDistribution distributes tokens to delegators in batches based on their computed scores.
// Uses TotalScore[ongoingID] for proportion calculation and iterates AccountScoreSnapshot[ongoingID].
//
// For each delegator in the batch:
//  1. Compute share: userAmount = totalPSEAmount × score / totalScore
//  2. Distribute via distributeToDelegator (send tokens + auto-delegate)
//  3. Track cumulative distributed amount
//  4. Remove the processed snapshot entry
//
// When all delegators have been processed, sends leftover (rounding errors + undelegated users) to the community pool.
// Returns true when distribution is complete and all state has been cleaned up.
func (k Keeper) ProcessPhase2TokenDistribution(
	ctx context.Context, ongoing types.ScheduledDistribution, bondDenom string,
) (bool, error) {
	ongoingID := ongoing.ID
	totalPSEAmount := getCommunityAllocationAmount(ongoing)

	totalScore, err := k.TotalScore.Get(ctx, ongoingID)
	if err != nil {
		return false, err
	}

	// No score or no amount: send everything to community pool and clean up.
	if !totalScore.IsPositive() || totalPSEAmount.IsZero() {
		if totalPSEAmount.IsPositive() {
			if err := k.sendLeftoverToCommunityPool(ctx, totalPSEAmount, bondDenom); err != nil {
				return false, err
			}
		}
		return true, k.cleanupDistribution(ctx, ongoingID)
	}

	// Collect a batch of score snapshots.
	iter, err := k.AccountScoreSnapshot.Iterate(
		ctx,
		collections.NewPrefixedPairRange[uint64, sdk.AccAddress](ongoingID),
	)
	if err != nil {
		return false, err
	}

	type scoreEntry struct {
		delAddr sdk.AccAddress
		score   sdkmath.Int
	}

	batch := make([]scoreEntry, 0, defaultBatchSize)
	for ; iter.Valid() && len(batch) < defaultBatchSize; iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			iter.Close()
			return false, err
		}
		batch = append(batch, scoreEntry{
			delAddr: kv.Key.K2(),
			score:   kv.Value,
		})
	}
	iter.Close()

	// Only triggered when all distributions of this round are completed.
	// Send leftover to community pool and clean up.
	if len(batch) == 0 {
		distributedSoFar, err := k.getDistributedAmount(ctx, ongoingID)
		if err != nil {
			return false, err
		}
		leftover := totalPSEAmount.Sub(distributedSoFar)
		if leftover.IsPositive() {
			if err := k.sendLeftoverToCommunityPool(ctx, leftover, bondDenom); err != nil {
				return false, err
			}
		}
		return true, k.cleanupDistribution(ctx, ongoingID)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Distribute rewards to each delegator in the batch proportional to their score.
	for _, item := range batch {
		userAmount := totalPSEAmount.Mul(item.score).Quo(totalScore)
		distributedAmount, err := k.distributeToDelegator(ctx, item.delAddr, userAmount, bondDenom)
		if err != nil {
			return false, err
		}

		if err := k.addToDistributedAmount(ctx, ongoingID, distributedAmount); err != nil {
			return false, err
		}

		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCommunityDistributed{
			DelegatorAddress: item.delAddr.String(),
			Score:            item.score,
			TotalPseScore:    totalScore,
			Amount:           userAmount,
			ScheduledAt:      ongoing.Timestamp,
		}); err != nil {
			sdkCtx.Logger().Error("failed to emit community distributed event", "error", err)
		}

		// Remove processed snapshot.
		if err := k.RemoveDelegatorScore(ctx, ongoingID, item.delAddr); err != nil {
			return false, err
		}
	}

	return false, nil
}

// getCommunityAllocationAmount extracts the community clearing account allocation from a distribution.
func getCommunityAllocationAmount(dist types.ScheduledDistribution) sdkmath.Int {
	for _, alloc := range dist.Allocations {
		if alloc.ClearingAccount == types.ClearingAccountCommunity {
			return alloc.Amount
		}
	}
	return sdkmath.NewInt(0)
}

// sendLeftoverToCommunityPool sends remaining undistributed tokens to the community pool.
func (k Keeper) sendLeftoverToCommunityPool(ctx context.Context, amount sdkmath.Int, bondDenom string) error {
	pseModuleAddress := k.accountKeeper.GetModuleAddress(types.ClearingAccountCommunity)
	return k.distributionKeeper.FundCommunityPool(ctx, sdk.NewCoins(sdk.NewCoin(bondDenom, amount)), pseModuleAddress)
}

// cleanupDistribution removes all state associated with a completed distribution.
func (k Keeper) cleanupDistribution(ctx context.Context, distributionID uint64) error {
	if err := k.AccountScoreSnapshot.Clear(
		ctx,
		collections.NewPrefixedPairRange[uint64, sdk.AccAddress](distributionID),
	); err != nil {
		return err
	}
	if err := k.TotalScore.Remove(ctx, distributionID); err != nil {
		return err
	}
	if err := k.DistributedAmount.Remove(ctx, distributionID); err != nil {
		return err
	}
	if err := k.AllocationSchedule.Remove(ctx, distributionID); err != nil {
		return err
	}
	if err := k.LastProcessedDistributionID.Set(ctx, distributionID); err != nil {
		return err
	}
	return k.OngoingDistribution.Remove(ctx)
}

// getDistributedAmount returns the cumulative distributed amount for a distribution, or zero if not set.
func (k Keeper) getDistributedAmount(ctx context.Context, distributionID uint64) (sdkmath.Int, error) {
	amount, err := k.DistributedAmount.Get(ctx, distributionID)
	if errors.Is(err, collections.ErrNotFound) {
		return sdkmath.NewInt(0), nil
	}
	return amount, err
}

// addToDistributedAmount atomically adds to the cumulative distributed amount.
func (k Keeper) addToDistributedAmount(ctx context.Context, distributionID uint64, amount sdkmath.Int) error {
	if amount.IsZero() {
		return nil
	}
	current, err := k.getDistributedAmount(ctx, distributionID)
	if err != nil {
		return err
	}
	return k.DistributedAmount.Set(ctx, distributionID, current.Add(amount))
}

func (k Keeper) distributeToDelegator(
	ctx context.Context, delAddr sdk.AccAddress, amount sdkmath.Int, bondDenom string,
) (sdkmath.Int, error) {
	if amount.IsZero() {
		return sdkmath.NewInt(0), nil
	}

	delAddrBech32, err := k.addressCodec.BytesToString(delAddr)
	if err != nil {
		return sdkmath.NewInt(0), err
	}
	delegationResponse, err := k.stakingKeeper.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delAddrBech32,
	})
	if err != nil {
		return sdkmath.NewInt(0), err
	}
	var delegations []stakingtypes.DelegationResponse
	totalDelegationAmount := sdkmath.NewInt(0)
	for _, delegation := range delegationResponse.DelegationResponses {
		delegations = append(delegations, delegation)
		totalDelegationAmount = totalDelegationAmount.Add(delegation.Balance.Amount)
	}

	// Send earned tokens to delegator's wallet regardless of active delegations.
	// Score was earned by staking during the scoring period, so the reward is always honored.
	if err = k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx,
		types.ClearingAccountCommunity,
		delAddr,
		sdk.NewCoins(sdk.NewCoin(bondDenom, amount)),
	); err != nil {
		return sdkmath.NewInt(0), err
	}

	// Auto-delegate proportionally to active validators. If no active delegations
	// (e.g., user undelegated during multi-block distribution), skip auto-delegation
	if len(delegations) == 0 || totalDelegationAmount.IsZero() {
		return amount, nil
	}

	for _, delegation := range delegations {
		// NOTE: this division will have rounding errors up to 1 subunit, which is acceptable and will be ignored.
		// if that one subunit exists, it will remain in user balance as undelegated.
		delegationAmount := delegation.Balance.Amount.Mul(amount).Quo(totalDelegationAmount)
		valAddr, err := k.valAddressCodec.StringToBytes(delegation.Delegation.ValidatorAddress)
		if err != nil {
			return sdkmath.NewInt(0), err
		}

		val, err := k.stakingKeeper.GetValidator(ctx, valAddr)
		if err != nil {
			return sdkmath.NewInt(0), err
		}

		_, err = k.stakingKeeper.Delegate(ctx, delAddr, delegationAmount, stakingtypes.Unbonded, val, true)
		if err != nil {
			return sdkmath.NewInt(0), err
		}
	}
	return amount, nil
}
