package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

// ConsumeOngoingDelegationTimeEntries processes a batch of DelegationTimeEntries
// from the ongoing distribution (ongoingID), converting each entry into a score
// snapshot and migrating it to nextID (ongoingID + 1).
//
// For each entry in the batch:
//  1. Calculate score from lastChanged to distribution timestamp -> addToMainScore(ongoingID)
//  2. Calculate gap score from distribution timestamp to current block time -> addToMainScore(nextID)
//  3. Create new entry under nextID with same shares, lastChanged = current block time
//  4. Remove entry from ongoingID
//
// Returns true when all ongoingID entries have been processed.
//
//nolint:funlen
func (k Keeper) ConsumeOngoingDelegationTimeEntries(
	ctx context.Context, ongoing types.ScheduledDistribution,
) (bool, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return false, err
	}
	batchSize := batchSizeFromParams(params)

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	ongoingID := ongoing.ID
	nextID := ongoing.ID + 1
	distTimestamp := int64(ongoing.Timestamp)
	blockTime := sdkCtx.BlockTime().Unix()

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

	batch := make([]entryKV, 0, batchSize)
	for ; iter.Valid() && len(batch) < batchSize; iter.Next() {
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

	if len(batch) == 0 {
		return true, nil
	}

	// Load excluded addresses once for the entire batch.
	excludedMap, err := k.LoadExcludedAddressMap(ctx)
	if err != nil {
		return false, err
	}

	for _, item := range batch {
		addrStr, err := k.addressCodec.BytesToString(item.delAddr)
		if err != nil {
			return false, errorsmod.Wrapf(err, "encode delegator bech32")
		}
		isExcluded := excludedMap[addrStr]

		// Score for ongoingID: lastChanged -> distTimestamp.
		// Excluded addresses route to ExcludedAddressScore instead of AccountScoreSnapshot+TotalScore.
		score, err := calculateScoreAtTimestamp(ctx, k, item.valAddr, item.entry, distTimestamp)
		if err != nil {
			return false, errorsmod.Wrapf(err,
				"phase 1 ongoing score: distribution_id=%d delegator=%s validator=%s last_changed=%d dist_ts=%d",
				ongoingID, addrStr, item.valAddr, item.entry.LastChangedUnixSec, distTimestamp)
		}
		if err := k.addScoreForAddress(ctx, ongoingID, item.delAddr, score, isExcluded); err != nil {
			return false, errorsmod.Wrapf(err,
				"phase 1 add ongoing score: distribution_id=%d delegator=%s score=%s excluded=%v",
				ongoingID, addrStr, score, isExcluded)
		}

		// Score for nextID: distTimestamp -> blockTime (gap during Phase 1 processing).
		gapScore, err := calculateScoreAtTimestamp(ctx, k, item.valAddr, types.DelegationTimeEntry{
			LastChangedUnixSec: distTimestamp,
			Shares:             item.entry.Shares,
		}, blockTime)
		if err != nil {
			return false, errorsmod.Wrapf(err,
				"phase 1 gap score: next_id=%d delegator=%s validator=%s dist_ts=%d block_ts=%d",
				nextID, addrStr, item.valAddr, distTimestamp, blockTime)
		}
		if err := k.addScoreForAddress(ctx, nextID, item.delAddr, gapScore, isExcluded); err != nil {
			return false, errorsmod.Wrapf(err,
				"phase 1 add gap score: next_id=%d delegator=%s score=%s excluded=%v",
				nextID, addrStr, gapScore, isExcluded)
		}

		// Migrate entry to nextID with same shares, lastChanged = current block time.
		if err := k.SetDelegationTimeEntry(ctx, nextID, item.valAddr, item.delAddr, types.DelegationTimeEntry{
			LastChangedUnixSec: blockTime,
			Shares:             item.entry.Shares,
		}); err != nil {
			return false, errorsmod.Wrapf(err,
				"phase 1 migrate entry to next_id: next_id=%d delegator=%s validator=%s",
				nextID, addrStr, item.valAddr)
		}

		// Remove from ongoingID.
		if err := k.RemoveDelegationTimeEntry(ctx, ongoingID, item.valAddr, item.delAddr); err != nil {
			return false, errorsmod.Wrapf(err,
				"phase 1 remove ongoing entry: distribution_id=%d delegator=%s validator=%s",
				ongoingID, addrStr, item.valAddr)
		}
	}

	// If batch was smaller than the limit, all entries have been consumed.
	return len(batch) < batchSize, nil
}

// ProcessOngoingTokenDistribution distributes tokens to delegators in batches based on their computed scores.
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
//
//nolint:funlen
func (k Keeper) ProcessOngoingTokenDistribution(
	ctx context.Context, ongoing types.ScheduledDistribution, bondDenom string,
) (bool, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return false, err
	}
	batchSize := batchSizeFromParams(params)

	ongoingID := ongoing.ID
	totalPSEAmount := getCommunityAllocationAmount(ongoing)

	totalScore, err := k.TotalScore.Get(ctx, ongoingID)
	if errors.Is(err, collections.ErrNotFound) {
		totalScore = sdkmath.NewInt(0)
	} else if err != nil {
		return false, err
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

	batch := make([]scoreEntry, 0, batchSize)
	for ; iter.Valid() && len(batch) < batchSize; iter.Next() {
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

	// Empty batch: distribution complete or no eligible recipients.
	// Finalize and refund any remaining intermediary balance to the community pool.
	if len(batch) == 0 {
		return true, k.finalizeCommunityDistribution(ctx, ongoing, totalPSEAmount, bondDenom)
	}

	// Invariant: non-empty snapshot with non-positive total score indicates a scoring bug.
	if totalPSEAmount.IsPositive() && !totalScore.IsPositive() {
		return false, errorsmod.Wrapf(
			types.ErrInvariantViolation,
			"positive PSE amount %s but non-positive total score %s for distribution %d",
			totalPSEAmount, totalScore, ongoingID,
		)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	nextID := ongoingID + 1
	blockTime := sdkCtx.BlockTime().Unix()
	// processingElapsedSec is the time elapsed since distribution began. Used for the fairness bonus below.
	// If StartedAt is not set (zero), bonus is skipped to stay backward-compatible.
	processingElapsedSec := blockTime - ongoing.StartedAt

	// Distribute rewards to each delegator in the batch proportional to their score.
	batchDistributed := sdkmath.NewInt(0)
	for _, item := range batch {
		userAmount := totalPSEAmount.Mul(item.score).Quo(totalScore)
		distributedAmount, err := k.distributeToDelegator(ctx, item.delAddr, userAmount, bondDenom)
		if err != nil {
			return false, errorsmod.Wrapf(err,
				"phase 2: distribution_id=%d delegator=%s score=%s user_amount=%s total_score=%s total_pse=%s",
				ongoingID, item.delAddr, item.score, userAmount, totalScore, totalPSEAmount)
		}
		batchDistributed = batchDistributed.Add(distributedAmount)

		// Fairness bonus: compensates delegators processed in later batches.
		// addToMainScore is safe here: AccountScoreSnapshot[ongoingID] only contains non-excluded addresses.
		if distributedAmount.IsPositive() && ongoing.StartedAt > 0 && processingElapsedSec > 0 {
			bonusScore := distributedAmount.MulRaw(processingElapsedSec)
			if err := k.addToMainScore(ctx, nextID, item.delAddr, bonusScore); err != nil {
				return false, errorsmod.Wrapf(err,
					"fairness bonus: next_id=%d delegator=%s bonus=%s",
					nextID, item.delAddr, bonusScore)
			}
		}

		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCommunityDistributed{
			DelegatorAddress: item.delAddr.String(),
			Score:            item.score,
			TotalPseScore:    totalScore,
			Amount:           distributedAmount,
			ScheduledAt:      ongoing.Timestamp,
			DistributionId:   ongoingID,
		}); err != nil {
			sdkCtx.Logger().Error("failed to emit community distributed event", "error", err)
		}

		// Remove processed snapshot.
		if err := k.RemoveDelegatorScore(ctx, ongoingID, item.delAddr); err != nil {
			return false, err
		}
	}

	if err := k.addToDistributedAmount(ctx, ongoingID, batchDistributed); err != nil {
		return false, err
	}

	return false, nil
}

// finalizeCommunityDistribution sends the undistributed leftover to the community pool, emits a
// finalization event, and cleans up all state for the completed distribution.
// Called when all AccountScoreSnapshot entries for the ongoing distribution have been processed.
func (k Keeper) finalizeCommunityDistribution(
	ctx context.Context, ongoing types.ScheduledDistribution, totalPSEAmount sdkmath.Int, bondDenom string,
) error {
	distributedSoFar, err := k.getDistributedAmount(ctx, ongoing.ID)
	if err != nil {
		return err
	}
	leftover := totalPSEAmount.Sub(distributedSoFar)
	if leftover.IsPositive() {
		if err := k.sendLeftoverToCommunityPool(ctx, leftover, bondDenom); err != nil {
			return err
		}
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCommunityDistributionFinalized{
		ScheduledAt:         ongoing.Timestamp,
		DistributionId:      ongoing.ID,
		TotalDistributed:    distributedSoFar,
		CommunityPoolAmount: leftover,
	}); err != nil {
		sdkCtx.Logger().Error("failed to emit community distributed finalized event", "error", err)
	}
	return k.cleanupOngoingDistribution(ctx, ongoing.ID)
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

// sendLeftoverToCommunityPool sends remaining undistributed tokens from the intermediary account to the community pool.
func (k Keeper) sendLeftoverToCommunityPool(ctx context.Context, amount sdkmath.Int, bondDenom string) error {
	intermediaryAddress := k.accountKeeper.GetModuleAddress(types.ClearingAccountCommunityIntermediary)
	return k.distributionKeeper.FundCommunityPool(ctx, sdk.NewCoins(sdk.NewCoin(bondDenom, amount)), intermediaryAddress)
}

// cleanupOngoingDistribution removes all state associated with a completed distribution.
func (k Keeper) cleanupOngoingDistribution(ctx context.Context, distributionID uint64) error {
	if err := k.AccountScoreSnapshot.Clear(
		ctx,
		collections.NewPrefixedPairRange[uint64, sdk.AccAddress](distributionID),
	); err != nil {
		return err
	}
	if err := k.TotalScore.Remove(ctx, distributionID); err != nil {
		return err
	}
	if err := k.ExcludedAddressScore.Clear(ctx, nil); err != nil {
		return err
	}
	if err := k.DistributedAmount.Remove(ctx, distributionID); err != nil {
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

//nolint:funlen
func (k Keeper) distributeToDelegator(
	ctx context.Context, delAddr sdk.AccAddress, amount sdkmath.Int, bondDenom string,
) (sdkmath.Int, error) {
	if amount.IsZero() {
		return sdkmath.NewInt(0), nil
	}

	delAddrBech32, err := k.addressCodec.BytesToString(delAddr)
	if err != nil {
		return sdkmath.NewInt(0), errorsmod.Wrapf(err, "encode delegator bech32")
	}
	delegationResponse, err := k.stakingKeeper.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delAddrBech32,
	})
	if err != nil {
		return sdkmath.NewInt(0), errorsmod.Wrapf(err,
			"query delegations: delegator=%s", delAddrBech32)
	}

	totalDelegationAmount := sdkmath.NewInt(0)
	for _, delegation := range delegationResponse.DelegationResponses {
		totalDelegationAmount = totalDelegationAmount.Add(delegation.Balance.Amount)
	}

	// Only distribute to users with active stakes. If not, it will be leftover.
	if totalDelegationAmount.IsZero() {
		return sdkmath.NewInt(0), nil
	}

	type eligibleDelegationEntry struct {
		delegation stakingtypes.DelegationResponse
		val        stakingtypes.Validator
		valAddr    sdk.ValAddress
	}

	var eligibleDelegations []eligibleDelegationEntry
	eligibleDelegationAmount := sdkmath.NewInt(0)

	for _, delegation := range delegationResponse.DelegationResponses {
		valAddr, err := k.valAddressCodec.StringToBytes(delegation.Delegation.ValidatorAddress)
		if err != nil {
			return sdkmath.NewInt(0), errorsmod.Wrapf(err,
				"decode validator address: delegator=%s validator=%s",
				delAddrBech32, delegation.Delegation.ValidatorAddress)
		}

		val, err := k.stakingKeeper.GetValidator(ctx, valAddr)
		if err != nil {
			return sdkmath.NewInt(0), errorsmod.Wrapf(err,
				"get validator: delegator=%s validator=%s",
				delAddrBech32, delegation.Delegation.ValidatorAddress)
		}

		if val.Jailed {
			// Jailed: ineligible for this distribution cycle.
			// Score kept; delegator participates in future distributions.
			continue
		}

		if delegation.Balance.Amount.IsZero() {
			// Skip fully-slashed validators (Balance=0) and auto-delegate only to healthy ones.
			// Otherwise SDK Delegate returns ErrDelegatorShareExRateInvalid and disables PSE.
			continue
		}

		eligibleDelegations = append(eligibleDelegations, eligibleDelegationEntry{
			delegation: delegation,
			val:        val,
			valAddr:    valAddr,
		})
		eligibleDelegationAmount = eligibleDelegationAmount.Add(delegation.Balance.Amount)
	}

	// Only eligibleShare is sent to the delegator and auto-delegated.
	if eligibleDelegationAmount.IsZero() {
		// All validators are jailed/slashed/inaccessible: bank-send is exactly
		// zero; full amount remains in intermediary => Community Pool.
		return sdkmath.NewInt(0), nil
	}

	eligibleShare := amount.Mul(eligibleDelegationAmount).Quo(totalDelegationAmount)
	if eligibleShare.IsZero() {
		return sdkmath.NewInt(0), nil
	}

	if err = k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx,
		types.ClearingAccountCommunityIntermediary,
		delAddr,
		sdk.NewCoins(sdk.NewCoin(bondDenom, eligibleShare)),
	); err != nil {
		return sdkmath.NewInt(0), errorsmod.Wrapf(err,
			"send reward from intermediary: delegator=%s amount=%s%s",
			delAddrBech32, amount, bondDenom)
	}

	// Auto-delegate eligibleShare proportionally across eligible validators only.
	for _, eligibleDelegation := range eligibleDelegations {
		if eligibleDelegation.delegation.Balance.Amount.IsZero() {
			continue
		}
		// NOTE: this division will have rounding errors up to 1 subunit, which is acceptable and will be ignored.
		// if that one subunit exists, it will remain in user balance as undelegated.
		delegationAmount := eligibleDelegation.delegation.Balance.Amount.Mul(eligibleShare).Quo(eligibleDelegationAmount)
		if delegationAmount.IsZero() {
			continue
		}

		_, err = k.stakingKeeper.Delegate(ctx, delAddr, delegationAmount, stakingtypes.Unbonded, eligibleDelegation.val, true)
		if err != nil {
			return sdkmath.NewInt(0), errorsmod.Wrapf(err,
				"auto-delegate: delegator=%s validator=%s amount=%s%s",
				delAddrBech32, eligibleDelegation.delegation.Delegation.ValidatorAddress, delegationAmount, bondDenom)
		}
	}

	return eligibleShare, nil
}

// batchSizeFromParams returns the configured batch size, falling back to the default
// when unset.
func batchSizeFromParams(params types.Params) int {
	if params.DistributionBatchSize == 0 {
		return int(types.DefaultParams().DistributionBatchSize)
	}
	return int(params.DistributionBatchSize)
}
