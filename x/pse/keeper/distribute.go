package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

// defaultBatchSize is the number of entries processed per EndBlock during multi-block distribution.
const defaultBatchSize = 100 // TODO: make configurable

// ProcessPhase1ScoreConversion processes a batch of DelegationTimeEntries from the ongoing distribution (prevID),
// converting each entry into a score snapshot and migrating it to currentID (prevID+1).
//
// For each entry in the batch:
//  1. Calculate score from lastChanged to distribution timestamp → addToScore(prevID)
//  2. Create new entry in currentID with same shares, lastChanged = distTimestamp
//  3. Remove entry from prevID
//
// Returns true when all prevID entries have been processed and TotalScore is computed.
func (k Keeper) ProcessPhase1ScoreConversion(ctx context.Context, ongoing types.ScheduledDistribution) (bool, error) {
	prevID := ongoing.ID
	currentID := ongoing.ID + 1
	distTimestamp := int64(ongoing.Timestamp)

	// Collect a batch of entries from prevID.
	iter, err := k.DelegationTimeEntries.Iterate(
		ctx,
		collections.NewPrefixedTripleRange[uint64, sdk.AccAddress, sdk.ValAddress](prevID),
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
		if err := k.computeTotalScore(ctx, prevID); err != nil {
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
			if err := k.addToScore(ctx, prevID, item.delAddr, score); err != nil {
				return false, err
			}
		}

		// Migrate entry to currentID with same shares, reset lastChanged to distribution timestamp.
		if err := k.SetDelegationTimeEntry(ctx, currentID, item.valAddr, item.delAddr, types.DelegationTimeEntry{
			LastChangedUnixSec: distTimestamp,
			Shares:             item.entry.Shares,
		}); err != nil {
			return false, err
		}

		// Remove from prevID.
		if err := k.RemoveDelegationTimeEntry(ctx, prevID, item.valAddr, item.delAddr); err != nil {
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

// DistributeCommunityPSE distributes the total community PSE amount to all delegators based on their score.
func (k Keeper) DistributeCommunityPSE(
	ctx context.Context,
	bondDenom string,
	totalPSEAmount sdkmath.Int,
	scheduledDistribution types.ScheduledDistribution,
) error {
	scheduledAt := scheduledDistribution.Timestamp
	// TODO update to use distribution ID, also consider period splits
	distributionID := scheduledDistribution.ID
	// iterate all delegation time entries and calculate uncalculated score.
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	finalScoreMap, err := newScoreMap(distributionID, k.addressCodec, params.ExcludedAddresses)
	if err != nil {
		return err
	}

	allDelegationTimeEntries, err := finalScoreMap.iterateDelegationTimeEntries(ctx, k)
	if err != nil {
		return err
	}

	// add uncalculated score to account score snapshot and total score per delegator.
	// it calculates the score from the last delegation time entry up to the current block time, which
	// is not included in the score snapshot calculations.
	err = finalScoreMap.iterateAccountScoreSnapshot(ctx, k)
	if err != nil {
		return err
	}

	// Clear all account score snapshots.
	// Excluded addresses should not have snapshots (cleared when added to exclusion list),
	// but we clear unconditionally for all addresses.
	// TODO review all the logic for score reset
	if err := k.AccountScoreSnapshot.Clear(
		ctx,
		collections.NewPrefixedPairRange[uint64, sdk.AccAddress](distributionID),
	); err != nil {
		return err
	}

	// reset all delegation time entries LastChangedUnixSec to the current block time.
	err = k.DelegationTimeEntries.Clear(
		ctx,
		collections.NewPrefixedTripleRange[uint64, sdk.AccAddress, sdk.ValAddress](distributionID),
	)
	if err != nil {
		return err
	}
	currentBlockTime := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	for _, kv := range allDelegationTimeEntries {
		kv.Value.LastChangedUnixSec = currentBlockTime
		// TODO review all the logic for score reset
		key := collections.Join3(distributionID+1, kv.Key.K2(), kv.Key.K3())
		err = k.DelegationTimeEntries.Set(ctx, key, kv.Value)
		if err != nil {
			return err
		}
	}

	// distribute total pse coin based on per delegator score.
	totalPSEScore := finalScoreMap.totalScore

	// leftover is the amount of pse coin that is not distributed to any delegator.
	// It will be sent to CommunityPool.
	// there are 2 sources of leftover:
	// 1. rounding errors due to division.
	// 2. some delegators have no delegation.
	leftover := totalPSEAmount
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if totalPSEScore.IsPositive() {
		err = finalScoreMap.walk(func(addr sdk.AccAddress, score sdkmath.Int) error {
			userAmount := totalPSEAmount.Mul(score).Quo(totalPSEScore)
			distributedAmount, err := k.distributeToDelegator(ctx, addr, userAmount, bondDenom)
			if err != nil {
				return err
			}
			leftover = leftover.Sub(distributedAmount)
			if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCommunityDistributed{
				DelegatorAddress: addr.String(),
				Score:            score,
				TotalPseScore:    totalPSEScore,
				Amount:           userAmount,
				ScheduledAt:      scheduledAt,
			}); err != nil {
				sdkCtx.Logger().Error("failed to emit community distributed event", "error", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// send leftover to CommunityPool.
	if leftover.IsPositive() {
		pseModuleAddress := k.accountKeeper.GetModuleAddress(types.ClearingAccountCommunity)
		err = k.distributionKeeper.FundCommunityPool(ctx, sdk.NewCoins(sdk.NewCoin(bondDenom, leftover)), pseModuleAddress)
		if err != nil {
			return err
		}
	}

	return nil
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

	if len(delegations) == 0 {
		return sdkmath.NewInt(0), nil
	}

	if err = k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx,
		types.ClearingAccountCommunity,
		delAddr,
		sdk.NewCoins(sdk.NewCoin(bondDenom, amount)),
	); err != nil {
		return sdkmath.NewInt(0), err
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
