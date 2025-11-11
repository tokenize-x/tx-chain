package keeper

import (
	"context"

	"cosmossdk.io/collections"
	addresscodec "cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// DistributeCommunityPSE distributes the total community PSE amount to all delegators based on their score.
func (k Keeper) DistributeCommunityPSE(ctx context.Context, totalPSEAmount sdkmath.Int) error {
	var allDelegationTimeEntryKeys []collections.Pair[sdk.ValAddress, sdk.AccAddress]
	// iterate all delegation time entries and calculate uncalculated score.
	var finalScoreMap = newScoreMap(k.addressCodec)
	delegationTimeEntriesIterator, err := k.DelegationTimeEntries.Iterate(ctx, nil)
	if err != nil {
		return err
	}
	defer delegationTimeEntriesIterator.Close()

	for ; delegationTimeEntriesIterator.Valid(); delegationTimeEntriesIterator.Next() {
		kv, err := delegationTimeEntriesIterator.KeyValue()
		if err != nil {
			return err
		}
		allDelegationTimeEntryKeys = append(allDelegationTimeEntryKeys, kv.Key)
		valAddr := kv.Key.K1()
		delAddr := kv.Key.K2()
		delegationTimeEntry := kv.Value
		delegationScore, err := calculateAddedScore(ctx, k, valAddr, delegationTimeEntry)
		if err != nil {
			return err
		}
		finalScoreMap.AddScore(delAddr, delegationScore)
	}

	// add uncalculated score to account score snapshot and total score per delegator.
	// it calculates the score from the last delegation time entry up to the current block time, which
	// is not included in the score snapshot calculations.
	iter, err := k.AccountScoreSnapshot.Iterate(ctx, nil)
	if err != nil {
		return err
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return err
		}
		score := kv.Value
		delAddr := kv.Key
		finalScoreMap.AddScore(delAddr, score)
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}

	// distribute total pse coin based on per delegator score.
	totalPSEScore := finalScoreMap.totalScore

	// leftover is the amount of pse coin that is not distributed to any delegator.
	// It will be sent to community module account.
	// there are 2 sources of leftover:
	// 1. rounding errors due to division.
	// 2. some delegators have no delegation.
	leftover := totalPSEAmount
	if totalPSEScore.IsPositive() {
		err = finalScoreMap.Walk(func(addr sdk.AccAddress, score sdkmath.Int) error {
			userAmount := totalPSEAmount.Mul(score).Quo(totalPSEScore)
			deliveredAmount, err := k.distributeToDelegator(ctx, addr, userAmount, bondDenom)
			if err != nil {
				return err
			}
			leftover = leftover.Sub(deliveredAmount)
			return nil
		})
		if err != nil {
			return err
		}
	}

	// send leftover to community module account.
	if leftover.IsPositive() {
		pseModuleAddress := k.accountKeeper.GetModuleAddress(k.getCommunityPSEClearingAccount())
		err = k.distributionKeeper.FundCommunityPool(ctx, sdk.NewCoins(sdk.NewCoin(bondDenom, leftover)), pseModuleAddress)
		if err != nil {
			return err
		}
	}

	// set all scores to 0.
	err = k.AccountScoreSnapshot.Clear(ctx, nil)
	if err != nil {
		return err
	}

	// set all delegation time entries to the current block time.
	blockTimeUnixSeconds := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	for _, key := range allDelegationTimeEntryKeys {
		err = k.DelegationTimeEntries.Set(ctx, key, types.DelegationTimeEntry{
			LastChangedUnixSec: blockTimeUnixSeconds,
			Shares:             sdkmath.LegacyNewDec(0),
		})
		if err != nil {
			return err
		}
	}

	return nil
}

type scoreMap struct {
	items []struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}
	indexMap     map[string]int
	addressCodec addresscodec.Codec
	totalScore   sdkmath.Int
}

func newScoreMap(addressCodec addresscodec.Codec) *scoreMap {
	return &scoreMap{
		items: make([]struct {
			addr  sdk.AccAddress
			score sdkmath.Int
		}, 0),
		indexMap:     make(map[string]int),
		addressCodec: addressCodec,
		totalScore:   sdkmath.NewInt(0),
	}
}

func (m *scoreMap) AddScore(addr sdk.AccAddress, value sdkmath.Int) {
	if value.IsZero() {
		return
	}
	key, err := m.addressCodec.BytesToString(addr)
	if err != nil {
		return
	}
	idx, found := m.indexMap[key]
	if !found {
		m.items = append(m.items, struct {
			addr  sdk.AccAddress
			score sdkmath.Int
		}{
			addr:  addr,
			score: value,
		})
		m.indexMap[key] = len(m.items) - 1
	} else {
		m.items[idx].score = m.items[idx].score.Add(value)
	}

	m.totalScore = m.totalScore.Add(value)
}

func (m *scoreMap) Walk(fn func(addr sdk.AccAddress, score sdkmath.Int) error) error {
	for _, pair := range m.items {
		if err := fn(pair.addr, pair.score); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) getCommunityPSEClearingAccount() string {
	return types.ClearingAccountCommunity
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
		k.getCommunityPSEClearingAccount(),
		delAddr,
		sdk.NewCoins(sdk.NewCoin(bondDenom, amount)),
	); err != nil {
		return sdkmath.NewInt(0), err
	}
	deliveredAmount := sdkmath.NewInt(0)
	for _, delegation := range delegations {
		// NOTE: this division will have rounding errors up to 1 subunit, which is acceptable and will be ignored.
		// the sum of all rounding errors will be sent to community module account.
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
		deliveredAmount = deliveredAmount.Add(delegationAmount)
	}
	return deliveredAmount, nil
}
