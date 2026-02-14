package keeper

import (
	"bytes"
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

const communityDistributionBatchSize uint64 = 1000

// DistributeCommunityPSE creates a community distribution job for batched payouts.
// The actual payouts are handled by ProcessCommunityDistributionBatch.
func (k Keeper) DistributeCommunityPSE(
	ctx context.Context,
	_ string,
	totalPSEAmount sdkmath.Int,
	scheduledAt uint64,
) error {
	return k.StartCommunityDistributionJob(ctx, totalPSEAmount, scheduledAt)
}

// StartCommunityDistributionJob snapshots scores and creates a new community distribution job.
func (k Keeper) StartCommunityDistributionJob(
	ctx context.Context,
	totalPSEAmount sdkmath.Int,
	scheduledAt uint64,
) error {
	if totalPSEAmount.IsZero() {
		return nil
	}

	_, err := k.CommunityJob.Get(ctx)
	if err == nil {
		return errorsmod.Wrap(types.ErrCommunityJobInProgress, "community job already exists")
	}
	if !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	finalScoreMap, err := newScoreMap(k.addressCodec, params.ExcludedAddresses)
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
	if err := k.AccountScoreSnapshot.Clear(ctx, nil); err != nil {
		return err
	}

	// reset all delegation time entries LastChangedUnixSec to the current block time.
	currentBlockTime := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	for _, kv := range allDelegationTimeEntries {
		kv.Value.LastChangedUnixSec = currentBlockTime
		err = k.DelegationTimeEntries.Set(ctx, kv.Key, kv.Value)
		if err != nil {
			return err
		}
	}

	if err := k.CommunityScores.Clear(ctx, nil); err != nil {
		return err
	}

	var totalEntries uint64
	err = finalScoreMap.walk(func(addr sdk.AccAddress, score sdkmath.Int) error {
		if score.IsZero() {
			return nil
		}
		if err := k.CommunityScores.Set(ctx, addr, score); err != nil {
			return err
		}
		totalEntries++
		return nil
	})
	if err != nil {
		return err
	}

	return k.CommunityJob.Set(ctx, types.CommunityDistributionJob{
		ScheduledAt:      scheduledAt,
		TotalAmount:      totalPSEAmount,
		TotalScore:       finalScoreMap.totalScore,
		Leftover:         totalPSEAmount,
		NextAddress:      "",
		TotalEntries:     totalEntries,
		ProcessedEntries: 0,
	})
}

// ProcessCommunityDistributionBatch processes a fixed-size batch of community payouts per block.
func (k Keeper) ProcessCommunityDistributionBatch(ctx context.Context, batchSize uint64) error {
	job, err := k.CommunityJob.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil
		}
		return err
	}

	if !job.TotalScore.IsPositive() || job.TotalEntries == 0 {
		return k.completeCommunityJob(ctx, job)
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}

	var nextAddrBytes []byte
	if job.NextAddress != "" {
		nextAddrBytes, err = k.addressCodec.StringToBytes(job.NextAddress)
		if err != nil {
			return err
		}
	}

	iter, err := k.CommunityScores.Iterate(ctx, nil)
	if err != nil {
		return err
	}
	defer iter.Close()

	processed := uint64(0)
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	for ; iter.Valid() && processed < batchSize; iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return err
		}
		if len(nextAddrBytes) > 0 && bytes.Compare(kv.Key, nextAddrBytes) <= 0 {
			continue
		}

		addr := kv.Key
		score := kv.Value
		userAmount := job.TotalAmount.Mul(score).Quo(job.TotalScore)
		distributedAmount, err := k.distributeToDelegator(ctx, addr, userAmount, bondDenom)
		if err != nil {
			return err
		}
		job.Leftover = job.Leftover.Sub(distributedAmount)
		job.ProcessedEntries++
		processed++

		addrStr, err := k.addressCodec.BytesToString(addr)
		if err != nil {
			return err
		}
		job.NextAddress = addrStr
		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventCommunityDistributed{
			DelegatorAddress: addrStr,
			Score:            score,
			TotalPseScore:    job.TotalScore,
			Amount:           userAmount,
			ScheduledAt:      job.ScheduledAt,
		}); err != nil {
			sdkCtx.Logger().Error("failed to emit community distributed event", "error", err)
		}
	}

	if job.ProcessedEntries >= job.TotalEntries {
		return k.completeCommunityJob(ctx, job)
	}

	return k.CommunityJob.Set(ctx, job)
}

func (k Keeper) completeCommunityJob(ctx context.Context, job types.CommunityDistributionJob) error {
	if job.Leftover.IsPositive() {
		bondDenom, err := k.stakingKeeper.BondDenom(ctx)
		if err != nil {
			return err
		}
		pseModuleAddress := k.accountKeeper.GetModuleAddress(types.ClearingAccountCommunity)
		if err := k.distributionKeeper.FundCommunityPool(
			ctx,
			sdk.NewCoins(sdk.NewCoin(bondDenom, job.Leftover)),
			pseModuleAddress,
		); err != nil {
			return err
		}
	}

	if err := k.CommunityScores.Clear(ctx, nil); err != nil {
		return err
	}

	return k.CommunityJob.Remove(ctx)
}

// CommunityDistributionBatchSize returns the default community distribution batch size per block.
func CommunityDistributionBatchSize() uint64 {
	return communityDistributionBatchSize
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
