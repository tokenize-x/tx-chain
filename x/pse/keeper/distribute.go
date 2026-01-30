package keeper

import (
	"context"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// DistributeCommunityPSE distributes the total community PSE amount to all delegators based on their score.
func (k Keeper) DistributeCommunityPSE(
	ctx context.Context,
	bondDenom string,
	totalPSEAmount sdkmath.Int,
	scheduledAt uint64,
) error {
	// iterate all delegation time entries and calculate uncalculated score.
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

		// TODO: do we need to get validators for each delegation? or we can get them beforehand?
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
