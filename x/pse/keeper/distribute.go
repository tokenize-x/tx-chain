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

func (k Keeper) Distribute(ctx context.Context, totalPSEAmount sdkmath.Int) error {
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
		dellAddr := kv.Key
		finalScoreMap.AddScore(dellAddr, score)
	}

	// distribute total pse coin based on per delegator score.
	totalPSEScore := finalScoreMap.totalScore
	err = finalScoreMap.Walk(func(addr sdk.AccAddress, score sdkmath.Int) error {
		userAmount := totalPSEAmount.Mul(score).Quo(totalPSEScore)
		err = k.distributeToDelegator(ctx, addr, userAmount)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
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
	m.totalScore = m.totalScore.Add(value)
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
}

func (m *scoreMap) Walk(fn func(addr sdk.AccAddress, score sdkmath.Int) error) error {
	for _, pair := range m.items {
		if err := fn(pair.addr, pair.score); err != nil {
			return err
		}
	}
	return nil
}

// TODO: fix the hardcoded module name.
func (k Keeper) getCommunityPSEModuleAccount() string {
	return "pse_community"
}

func (k Keeper) distributeToDelegator(ctx context.Context, delAddr sdk.AccAddress, amount sdkmath.Int) error {
	delAddrBech32, err := k.addressCodec.BytesToString(delAddr)
	if err != nil {
		return err
	}
	delegationResponse, err := k.stakingKeeper.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delAddrBech32,
	})
	if err != nil {
		return err
	}
	var delegations []stakingtypes.DelegationResponse
	totalDelegationAmount := sdkmath.NewInt(0)
	for _, delegation := range delegationResponse.DelegationResponses {
		delegations = append(delegations, delegation)
		totalDelegationAmount = totalDelegationAmount.Add(delegation.Balance.Amount)
	}

	if len(delegations) == 0 {
		return nil
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}
	if err = k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx,
		k.getCommunityPSEModuleAccount(),
		delAddr,
		sdk.NewCoins(sdk.NewCoin(bondDenom, amount)),
	); err != nil {
		return err
	}
	for _, delegation := range delegations {
		delegationAmount := delegation.Balance.Amount.Mul(amount).Quo(totalDelegationAmount)
		valAddr, err := k.valAddressCodec.StringToBytes(delegation.Delegation.ValidatorAddress)
		if err != nil {
			return err
		}

		val, err := k.stakingKeeper.GetValidator(ctx, valAddr)
		if err != nil {
			return err
		}

		_, err = k.stakingKeeper.Delegate(ctx, delAddr, delegationAmount, stakingtypes.Unbonded, val, true)
		if err != nil {
			return err
		}
	}
	return nil
}
